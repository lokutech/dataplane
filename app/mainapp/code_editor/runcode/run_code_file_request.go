package runcode

import (
	"dataplane/mainapp/code_editor/filesystem"
	"dataplane/mainapp/database"
	"dataplane/mainapp/database/models"
	"dataplane/mainapp/logging"
	"dataplane/mainapp/messageq"
	"dataplane/mainapp/utilities"
	"dataplane/workers/config"
	"dataplane/workers/runtask"
	"encoding/json"
	"errors"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Command struct {
	Command string `json:command`
}

/*
Task status: Queue, Allocated, Started, Failed, Success
*/
func RunCodeFile(workerGroup string, fileID string, envID string, pipelineID string, nodeID string, nodeTypeDesc string, runid string) (models.CodeRun, error) {

	// Important not to update status to avoid timing issue where it can overwrite a success a status
	if runid == "" {
		runid = uuid.NewString()
	}
	// ------ Obtain folder structure from file id

	filesdata := models.CodeFiles{}
	database.DBConn.Where("file_id = ? and environment_id =? and level = ?", fileID, envID, "node_file").Find(&filesdata)

	parentfolderdata := ""
	var err error
	if filesdata.FolderID != "" {
		parentfolderdata, err = filesystem.FolderConstructByID(database.DBConn, filesdata.FolderID, envID, "pipelines")
		if err != nil {
			return models.CodeRun{}, errors.New("File record not found")
		}
	} else {
		return models.CodeRun{}, errors.New("File record not found")
	}

	// Map folder structure:
	var folderMap string
	var folderIDMap string

	folderMap = parentfolderdata
	folderIDMap = filesdata.FolderID
	if config.Debug == "yes" {
		if _, err := os.Stat(config.CodeDirectory + folderMap); os.IsExist(err) {
			log.Println("Dir exists:", config.CodeDirectory+folderMap)

		}
	}

	// ------ Construct run command
	var commands []string
	switch nodeTypeDesc {
	case "python":
		commands = append(commands, "python3 -u ${{nodedirectory}}"+filesdata.FileName)
	default:
		return models.CodeRun{}, errors.New("Code run type not found.")
	}

	/* Look up chosen workers -
	if none, keep trying for 10 x 2 seconds
	before failing */
	// var err1 error
	runSend := models.CodeRun{}
	markFail := true
	maxRetiresAllowed := 5
	var onlineWorkers []models.WorkerStats
	for i := 0; i < maxRetiresAllowed; i++ {

		// log.Println(i)

		var workers []models.Workers
		var worker models.WorkerStats

		err := database.DBConn.Where("environment_id =? and worker_group = ?", envID, workerGroup).Find(&workers).Error
		if err != nil && err != gorm.ErrRecordNotFound {
			return models.CodeRun{}, errors.New("Code run: Worker groups database error.")
		}

		for _, v := range workers {

			worker = models.WorkerStats{
				WorkerGroup: v.WorkerGroup,
				WorkerID:    v.WorkerID,
				Status:      v.Status,
				CPUPerc:     v.CPUPerc,
				Load:        v.Load,
				MemoryPerc:  v.MemoryPerc,
				MemoryUsed:  v.MemoryUsed,
				EnvID:       v.EnvironmentID,
				LB:          v.LB,
				WorkerType:  v.WorkerType,
			}

			onlineWorkers = append(onlineWorkers, worker)

		}

		// log.Println("X:", onlineWorkers)

		// log.Println("err1:", err1)

		if len(onlineWorkers) == 0 {
			log.Println(workerGroup + " not online, retrying in 2 seconds (" + strconv.Itoa(i) + " of " + strconv.Itoa(maxRetiresAllowed) + ")")
		} else {

			// Choose a worker based on load balancing strategy - default is round robin
			var loadbalanceNext string

			// if a worker group goes offline in between, choose the next in the load balancer and retry

			if config.Debug == "true" {
				log.Println("Worker LB:", onlineWorkers[0].LB, onlineWorkers)
			}

			switch onlineWorkers[0].LB {
			case "roundrobin":
				loadbalanceNext = utilities.Balance(onlineWorkers, workerGroup)

			default:
				loadbalanceNext = utilities.Balance(onlineWorkers, workerGroup)

			}

			// Send the request to the worker
			if config.Debug == "true" {
				log.Println("Selected worker:", onlineWorkers[0].LB, loadbalanceNext)
			}

			commandsprep := []Command{}
			for _, v := range commands {
				commandsprep = append(commandsprep, Command{Command: v})
			}

			commandJSON, err := json.Marshal(commandsprep)
			if err != nil {
				logging.PrintSecretsRedact(err)
			}

			runSend = models.CodeRun{
				RunID:         runid,
				NodeID:        nodeID,
				FileID:        filesdata.FileID,
				CreatedAt:     time.Now().UTC(),
				EnvironmentID: envID,
				WorkerGroup:   workerGroup,
				WorkerID:      loadbalanceNext,
				Commands:      commandJSON,
				Status:        "Queue",
				Folder:        folderMap,
				FolderID:      folderIDMap,
			}

			err2 := database.DBConn.Create(&runSend)
			if err2.Error != nil {
				logging.PrintSecretsRedact(err2.Error.Error())
			}

			var response runtask.TaskResponse

			// log.Println("Task channel: ", "task."+workerGroup+"."+loadbalanceNext)
			channel := "runcodefile." + workerGroup + "." + loadbalanceNext
			// log.Println(channel)
			_, errnats := messageq.MsgReply(channel, runSend, &response)

			if errnats != nil {
				log.Println("Send to worker error nats:", errnats)
			}

			// successful send to worker
			if response.R == "ok" {
				markFail = false
				break
			}

			if response.R != "ok" {
				markFail = true
				break
			}
			// } else {
			// 	log.Println(loadbalanceNext + " not online, retrying in 2 seconds (" + strconv.Itoa(i) + " of " + strconv.Itoa(maxRetiresAllowed) + ")")
			// }
			if config.Debug == "true" {
				log.Println("Send to worker", response.R, response.M)
			}
		}
		time.Sleep(2 * time.Second)
	}

	// log.Println("Mark as fail:", markFail)

	// If task not successfully sent, mark as failed
	updatedt := time.Now().UTC()
	if markFail {
		TaskFinal := models.CodeRun{
			EnvironmentID: envID,
			RunID:         runid,
			WorkerGroup:   workerGroup,
			UpdatedAt:     &updatedt,
			EndedAt:       time.Now().UTC(),
			Status:        "Fail",
			Reason:        "No workers",
		}

		UpdateCodeRun(TaskFinal)
		response := runtask.TaskResponse{R: "ok"}
		_, errnats := messageq.MsgReply("coderunupdate", TaskFinal, &response)
		if errnats != nil {
			logging.PrintSecretsRedact(errnats)
			return models.CodeRun{}, errors.New("No workers available")
		}

		runSend.Status = TaskFinal.Status
		runSend.Reason = TaskFinal.Reason

		// err3 := database.DBConn.Model(&models.WorkerTasks{}).Where("run_id = ? and status=?", runid, "Queue").Updates(map[string]interface{}{"status": "Fail", "reason": "Upstream no workers"}).Error
		// if err3 != nil {
		// 	logging.PrintSecretsRedact(err3.Error())
		// }

	}

	//
	return runSend, nil

}
