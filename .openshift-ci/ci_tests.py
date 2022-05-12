#!/usr/bin/env python3

"""
Available tests
"""

import subprocess

from common import popen_graceful_kill

# Where the QA tests store failure logs: qa-tests-backend/src/main/groovy/common/Constants.groovy
QA_TESTS_OUTPUT_DIR = "/tmp/qa-tests-backend-logs"


class BaseTest:
    def __init__(self):
        self.test_output_dirs = []

    def run_with_graceful_kill(self, args, timeout, post_start_hook=None):
        with subprocess.Popen(args) as cmd:
            if post_start_hook is not None:
                post_start_hook()
            try:
                exitstatus = cmd.wait(timeout)
                if exitstatus != 0:
                    raise RuntimeError(f"Test failed: exit {exitstatus}")
            except subprocess.TimeoutExpired as err:
                popen_graceful_kill(cmd)
                raise err


class NullTest(BaseTest):
    def run(self):
        pass


class UpgradeTest(BaseTest):
    TEST_TIMEOUT = 60 * 60
    TEST_OUTPUT_DIR = "/tmp/upgrade-test-logs"

    def run(self):
        print("Executing the Upgrade Test")

        def set_dirs_after_start():
            # let post test know where logs are
            self.test_output_dirs = [UpgradeTest.TEST_OUTPUT_DIR, QA_TESTS_OUTPUT_DIR]

        self.run_with_graceful_kill(
            ["tests/upgrade/run.sh", UpgradeTest.TEST_OUTPUT_DIR],
            UpgradeTest.TEST_TIMEOUT,
            post_start_hook=set_dirs_after_start,
        )


class QaE2eTestPart1(BaseTest):
    TEST_TIMEOUT = 240 * 60

    def run(self):
        print("Executing qa-tests-backend tests (part I)")

        self.run_with_graceful_kill(
            ["qa-tests-backend/scripts/run-part-1.sh"], QaE2eTestPart1.TEST_TIMEOUT
        )


class QaE2eTestPart2(BaseTest):
    TEST_TIMEOUT = 30 * 60

    def run(self):
        print("Executing qa-tests-backend tests (part II)")

        self.run_with_graceful_kill(
            ["qa-tests-backend/scripts/run-part-2.sh"], QaE2eTestPart2.TEST_TIMEOUT
        )
