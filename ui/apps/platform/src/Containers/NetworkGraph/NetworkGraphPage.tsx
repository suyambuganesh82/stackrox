import React, { useState, useEffect } from 'react';
import {
    PageSection,
    Title,
    Flex,
    FlexItem,
    Bullseye,
    Spinner,
    Button,
    Divider,
    Toolbar,
    ToolbarContent,
    ToolbarGroup,
    ToolbarItem,
} from '@patternfly/react-core';
import { EdgeModel } from '@patternfly/react-topology';
import useDeepCompareEffect from 'use-deep-compare-effect';

import useFetchClusters from 'hooks/useFetchClusters';
import useFetchDeploymentCount from 'hooks/useFetchDeploymentCount';
import useURLSearch from 'hooks/useURLSearch';
import { fetchNetworkFlowGraph, fetchNetworkPolicyGraph } from 'services/NetworkService';
import { getQueryString } from 'utils/queryStringUtils';
import timeWindowToDate from 'utils/timeWindows';

import PageTitle from 'Components/PageTitle';
import useURLParameter from 'hooks/useURLParameter';
import EmptyUnscopedState from './components/EmptyUnscopedState';
import NetworkBreadcrumbs from './components/NetworkBreadcrumbs';
import SimulateNetworkPolicyButton from './simulation/SimulateNetworkPolicyButton';
import EdgeStateSelect, { EdgeState } from './EdgeStateSelect';
import NetworkGraph from './NetworkGraph';
import {
    transformPolicyData,
    transformActiveData,
    createExtraneousFlowsModel,
    graphModel,
} from './utils/modelUtils';
import getScopeHierarchy from './utils/getScopeHierarchy';
import getSimulation from './utils/getSimulation';
import { CustomModel, CustomNodeModel, DeploymentNodeModel } from './types/topology.type';

import './NetworkGraphPage.css';

const emptyModel = {
    graph: graphModel,
};

// TODO: get real time window from user input
const timeWindow = 'Past hour';
// TODO: get real includePorts flag from user input
const includePorts = true;

function NetworkGraphPage() {
    const [edgeState, setEdgeState] = useState<EdgeState>('active');
    const [activeModel, setActiveModel] = useState<CustomModel>(emptyModel);
    const [extraneousFlowsModel, setExtraneousFlowsModel] = useState<CustomModel>(emptyModel);
    const [model, setModel] = useState<CustomModel>(emptyModel);
    const [isLoading, setIsLoading] = useState(false);
    const { searchFilter } = useURLSearch();
    const [simulationQueryValue] = useURLParameter('simulation', undefined);

    const {
        cluster: clusterFromUrl,
        namespaces: namespacesFromUrl,
        deployments: deploymentsFromUrl,
        remainingQuery,
    } = getScopeHierarchy(searchFilter);
    const simulation = getSimulation(simulationQueryValue);

    const hasClusterNamespaceSelected = Boolean(clusterFromUrl && namespacesFromUrl.length);

    const { clusters } = useFetchClusters();
    const selectedClusterId = clusters.find((cl) => cl.name === clusterFromUrl)?.id;
    const selectedCluster = { name: clusterFromUrl, id: selectedClusterId };
    const { deploymentCount } = useFetchDeploymentCount(selectedClusterId || '');

    useDeepCompareEffect(() => {
        // only refresh the graph data from the API if both a cluster and at least one namespace are selected
        if (clusterFromUrl && namespacesFromUrl.length > 0 && deploymentCount) {
            if (selectedClusterId) {
                setIsLoading(true);

                const queryToUse = getQueryString(remainingQuery).slice(1);
                const timestampToUse = timeWindowToDate(timeWindow);

                Promise.all([
                    fetchNetworkFlowGraph(
                        selectedClusterId,
                        namespacesFromUrl,
                        deploymentsFromUrl,
                        queryToUse,
                        timestampToUse || undefined,
                        includePorts
                    ),
                    fetchNetworkPolicyGraph(
                        selectedClusterId,
                        namespacesFromUrl,
                        deploymentsFromUrl,
                        queryToUse,
                        undefined,
                        includePorts
                    ),
                ])
                    .then((values) => {
                        const activeNodeMap: Record<string, CustomNodeModel> = {};
                        const activeEdgeMap: Record<string, EdgeModel> = {};
                        const policyNodeMap: Record<string, DeploymentNodeModel> = {};

                        // get policy nodes from api response
                        const { nodes: policyNodes } = values[1].response;
                        // transform policy data to DataModel
                        const policyDataModel = transformPolicyData(
                            policyNodes,
                            deploymentCount || 0
                        );
                        // set policyNodeMap to be able to cross reference nodes by id
                        // to enhance active node data
                        policyDataModel.nodes?.forEach((node) => {
                            // no grouped nodes in policy graph data model
                            if (!policyNodeMap[node.id]) {
                                policyNodeMap[node.id] = node as DeploymentNodeModel;
                            }
                        });

                        // get active nodes from api response
                        const { nodes: activeNodes } = values[0].response;
                        // transform active data to DataModel
                        const activeDataModel = transformActiveData(activeNodes, policyNodeMap);
                        // set activeNodeMap to be able to cross reference nodes by id
                        // for the extraneous graph
                        activeDataModel.nodes?.forEach((node) => {
                            // only add to node map when it's not a group node
                            if (!activeNodeMap[node.id] && !node.group) {
                                activeNodeMap[node.id] = node;
                            }
                        });
                        // set activeEdgeMap to be able to cross reference edges by id
                        // for the extraneous graph
                        activeDataModel.edges?.forEach((edge) => {
                            if (!activeEdgeMap[edge.id]) {
                                activeEdgeMap[edge.id] = edge;
                            }
                        });

                        // create extraneous flows graph
                        const extraneousFlowsDataModel = createExtraneousFlowsModel(
                            policyDataModel,
                            activeNodeMap,
                            activeEdgeMap
                        );
                        setActiveModel(activeDataModel);
                        setExtraneousFlowsModel(extraneousFlowsDataModel);
                    })
                    .catch(() => {
                        // TODO
                    })
                    .finally(() => setIsLoading(false));
            }
        }
    }, [clusters, clusterFromUrl, namespacesFromUrl, deploymentsFromUrl, deploymentCount]);

    useEffect(() => {
        if (edgeState === 'active') {
            setModel(activeModel);
        } else if (edgeState === 'extraneous') {
            setModel(extraneousFlowsModel);
        }
    }, [edgeState, setModel, activeModel, extraneousFlowsModel]);

    return (
        <>
            <PageTitle title="Network Graph" />
            <PageSection variant="light">
                <Flex alignItems={{ default: 'alignItemsCenter' }}>
                    <FlexItem>
                        <Title headingLevel="h1" className="pf-u-screen-reader">
                            Network Graph
                        </Title>
                    </FlexItem>
                    <FlexItem flex={{ default: 'flex_1' }}>
                        <NetworkBreadcrumbs
                            clusters={clusters}
                            selectedCluster={selectedCluster}
                            selectedNamespaces={namespacesFromUrl}
                            selectedDeployments={deploymentsFromUrl}
                        />
                    </FlexItem>
                    <Button variant="secondary">Manage CIDR blocks</Button>
                    <SimulateNetworkPolicyButton simulation={simulation} />
                </Flex>
            </PageSection>
            <Divider component="div" />
            <PageSection variant="light" padding={{ default: 'noPadding' }}>
                <Toolbar data-testid="network-graph-toolbar">
                    <ToolbarContent>
                        <ToolbarGroup variant="filter-group">
                            <ToolbarItem>
                                <EdgeStateSelect
                                    edgeState={edgeState}
                                    setEdgeState={setEdgeState}
                                />
                            </ToolbarItem>
                            <ToolbarItem>in the past hour</ToolbarItem>
                        </ToolbarGroup>
                        <ToolbarGroup>
                            <ToolbarItem>Add one or more deployment filters</ToolbarItem>
                            <ToolbarItem>Display options</ToolbarItem>
                        </ToolbarGroup>
                        <ToolbarGroup alignment={{ default: 'alignRight' }}>
                            <Divider component="div" isVertical />
                            <ToolbarItem>Last updated at 12:34PM</ToolbarItem>
                        </ToolbarGroup>
                    </ToolbarContent>
                </Toolbar>
            </PageSection>
            <Divider component="div" />
            <PageSection
                className="network-graph"
                variant={hasClusterNamespaceSelected ? 'default' : 'light'}
                padding={{ default: 'noPadding' }}
            >
                {!hasClusterNamespaceSelected && <EmptyUnscopedState />}
                {model.nodes && (
                    <NetworkGraph
                        model={model}
                        edgeState={edgeState}
                        simulation={simulation}
                        selectedClusterId={selectedClusterId || ''}
                    />
                )}
                {isLoading && (
                    <Bullseye>
                        <Spinner isSVG />
                    </Bullseye>
                )}
            </PageSection>
        </>
    );
}

export default NetworkGraphPage;
