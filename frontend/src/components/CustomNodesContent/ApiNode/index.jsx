import { faGlobe } from '@fortawesome/free-solid-svg-icons';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';
import { Grid, Tooltip, Typography, useTheme } from '@mui/material';
import { Box } from '@mui/system';
import { useEffect, useState } from 'react';
import { Handle } from 'react-flow-renderer';
import { useGlobalFlowState } from '../../../pages/PipelineEdit';
import { useGlobalRunState } from '../../../pages/PipelineRuns/GlobalRunState';
import customNodeStyle from '../../../utils/customNodeStyle';
import { customSourceHandle, customSourceHandleDragging } from '../../../utils/handleStyles';
import ApiTriggerNodeItem from '../../MoreInfoContent/APITriggerNodeItem';
import MoreInfoMenu from '../../MoreInfoMenu';
import { getColor } from '../utils';

const ApiNode = (props) => {
    // Theme hook
    const theme = useTheme();

    // Global state
    const FlowState = useGlobalFlowState();
    const RunState = useGlobalRunState();

    const [isEditorPage, setIsEditorPage] = useState(false);
    const [, setIsSelected] = useState(false);
    const [borderColor, setBorderColor] = useState('#c4c4c4');

    useEffect(() => {
        setIsEditorPage(FlowState.isEditorPage.get());
        setIsSelected(FlowState.selectedElement.get()?.id === props.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    useEffect(() => {
        setIsEditorPage(FlowState.isEditorPage.get());
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [FlowState.isEditorPage.get()]);

    useEffect(() => {
        setIsSelected(FlowState.selectedElement.get()?.id === props.id);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [FlowState.selectedElement.get()]);

    // Set border color on node status change
    useEffect(() => {
        setBorderColor(getColor(RunState[props.id]?.status?.get()));

        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [RunState[props.id].status?.get()]);

    return (
        <Box sx={{ ...customNodeStyle, border: `3px solid ${borderColor}` }}>
            <Handle type="source" position="right" id="schedule" style={FlowState.isDragging.get() ? customSourceHandleDragging : customSourceHandle(theme.palette.mode)} />

            <Grid container alignItems="flex-start" wrap="nowrap">
                <Box component={FontAwesomeIcon} fontSize={19} color="secondary.main" icon={faGlobe} />
                <Tooltip title={'Node ID: ' + props.id} placement="top">
                    <Grid item ml={1.5} textAlign="left">
                        <Typography fontSize={11} fontWeight={900}>
                            API trigger
                        </Typography>

                        <Typography fontSize={10} mt={1}>
                            Receive webhooks and API calls.
                        </Typography>
                    </Grid>
                </Tooltip>
            </Grid>

            {isEditorPage && (
                <Grid position="absolute" bottom={2} right={9} container wrap="nowrap" width="auto" alignItems="center" justifyContent="space-between">
                    <Box mt={2}>
                        <MoreInfoMenu iconHorizontal iconColor="#0073C6" iconColorDark="#0073C6" iconSize={19} noPadding>
                            <ApiTriggerNodeItem />
                        </MoreInfoMenu>
                    </Box>
                </Grid>
            )}
        </Box>
    );
};

export default ApiNode;
