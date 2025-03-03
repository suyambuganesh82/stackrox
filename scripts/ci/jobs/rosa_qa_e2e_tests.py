#!/usr/bin/env -S python3 -u

"""
Run qa-tests-backend in a ROSA cluster provided via automation-flavors/rosa.
"""
import os
from base_qa_e2e_test import make_qa_e2e_test_runner
from clusters import AutomationFlavorsCluster

# set required test parameters
os.environ["ORCHESTRATOR_FLAVOR"] = "openshift"
os.environ["SENSOR_HELM_DEPLOY"] = "true"

# don't use postgres
os.environ["ROX_POSTGRES_DATASTORE"] = "false"

make_qa_e2e_test_runner(cluster=AutomationFlavorsCluster()).run()
