import React from 'react';
import { Button } from '@patternfly/react-core';
import { useHistory } from 'react-router-dom';

import { networkBasePathPF } from 'routePaths';
import useURLParameter from 'hooks/useURLParameter';
import { Simulation } from '../utils/getSimulation';

type SimulateNetworkPolicyButtonProps = {
    simulation: Simulation;
};

function SimulateNetworkPolicyButton({ simulation }: SimulateNetworkPolicyButtonProps) {
    const history = useHistory();

    const [, setSimulationQueryValue] = useURLParameter('simulation', undefined);

    function enableNetworkPolicySimulation() {
        history.push({
            path: networkBasePathPF,
            search: history.location.search,
        });
        setSimulationQueryValue('networkPolicy');
    }

    return (
        <Button
            variant="secondary"
            isDisabled={simulation.isOn}
            onClick={enableNetworkPolicySimulation}
        >
            Simulate network policy
        </Button>
    );
}

export default SimulateNetworkPolicyButton;
