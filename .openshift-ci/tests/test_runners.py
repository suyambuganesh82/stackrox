import unittest
from unittest.mock import Mock
from runners import ClusterTestRunner, ClusterTestSetsRunner


class TestClusterTestRunner(unittest.TestCase):
    def test_provisions(self):
        cluster = Mock()
        ClusterTestRunner(cluster=cluster).run()
        cluster.provision.assert_called_once()

    def test_runs_pre_test(self):
        pre_test = Mock()
        ClusterTestRunner(pre_test=pre_test).run()
        pre_test.run.assert_called_once()

    def test_runs_test(self):
        test = Mock()
        ClusterTestRunner(test=test).run()
        test.run.assert_called_once()

    def test_runs_post_test(self):
        post_test = Mock()
        ClusterTestRunner(post_test=post_test).run()
        post_test.run.assert_called_once()

    def test_tearsdown(self):
        cluster = Mock()
        ClusterTestRunner(cluster=cluster).run()
        cluster.teardown.assert_called_once()

    def test_provision_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        cluster.provision.side_effect = Exception("oops")
        with self.assertRaisesRegex(Exception, "oops"):
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()
        test.run.assert_not_called()  # skips test
        post_test.run.assert_not_called()  # skips post test
        cluster.teardown.assert_called_once()  # still tearsdown

    def test_pre_test_failure(self):
        cluster = Mock()
        pre_test = Mock()
        test = Mock()
        post_test = Mock()
        pre_test.run.side_effect = Exception("oops")
        with self.assertRaisesRegex(Exception, "oops"):
            ClusterTestRunner(
                cluster=cluster, pre_test=pre_test, test=test, post_test=post_test
            ).run()
        test.run.assert_not_called()  # skips test
        post_test.run.assert_not_called()  # skips post test
        cluster.teardown.assert_called_once()  # still tearsdown

    def test_run_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        test.run.side_effect = Exception("oops")
        with self.assertRaisesRegex(Exception, "oops"):
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()
        test.run.assert_called_once()  # skips test
        post_test.run.assert_called_once()  # still post tests
        cluster.teardown.assert_called_once()  # still tearsdown

    def test_post_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        post_test.run.side_effect = Exception("oops")
        with self.assertRaisesRegex(Exception, "oops"):
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()
        cluster.teardown.assert_called_once()  # still tearsdown

    def test_run_and_post_test_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        test.run.side_effect = Exception("run oops")
        post_test.run.side_effect = Exception("post test oops")
        with self.assertRaisesRegex(Exception, "run oops"):  # the run error is #1
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()
        cluster.teardown.assert_called_once()  # still tearsdown

    def test_run_and_post_test_and_teardown_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        test.run.side_effect = Exception("run oops")
        post_test.run.side_effect = Exception("post test oops")
        cluster.teardown.side_effect = Exception("teardown oops")
        with self.assertRaisesRegex(Exception, "run oops"):  # the run error is #1
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()

    def test_post_test_and_teardown_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        post_test.run.side_effect = Exception("post test oops")
        cluster.teardown.side_effect = Exception("teardown oops")
        with self.assertRaisesRegex(
            Exception, "post test oops"
        ):  # the post_test error is #1
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()

    def test_provision_and_teardown_failure(self):
        cluster = Mock()
        test = Mock()
        post_test = Mock()
        cluster.provision.side_effect = Exception("provision oops")
        cluster.teardown.side_effect = Exception("teardown oops")
        with self.assertRaisesRegex(
            Exception, "provision oops"
        ):  # the provision error is #1
            ClusterTestRunner(cluster=cluster, test=test, post_test=post_test).run()

    def test_pre_test_and_teardown_failure(self):
        cluster = Mock()
        pre_test = Mock()
        test = Mock()
        post_test = Mock()
        pre_test.run.side_effect = Exception("pre test oops")
        cluster.teardown.side_effect = Exception("teardown oops")
        with self.assertRaisesRegex(
            Exception, "pre test oops"
        ):  # the pre test error is #1
            ClusterTestRunner(
                cluster=cluster, pre_test=pre_test, test=test, post_test=post_test
            ).run()


class TestClusterTestSetsRunner(unittest.TestCase):
    def test_provisions(self):
        cluster = Mock()
        ClusterTestSetsRunner(cluster=cluster).run()
        cluster.provision.assert_called_once()

    def test_tearsdown(self):
        cluster = Mock()
        ClusterTestSetsRunner(cluster=cluster).run()
        cluster.teardown.assert_called_once()

    def test_runs_pre_test(self):
        pre_test = Mock()
        ClusterTestSetsRunner(sets=[{"pre_test": pre_test}]).run()
        pre_test.run.assert_called_once()

    def test_runs_test(self):
        test = Mock()
        ClusterTestSetsRunner(sets=[{"test": test}]).run()
        test.run.assert_called_once()

    def test_runs_post_test(self):
        post_test = Mock()
        ClusterTestSetsRunner(sets=[{"post_test": post_test}]).run()
        post_test.run.assert_called_once()

    def test_runs_nth_pre_test(self):
        pre_test = Mock()
        ClusterTestSetsRunner(sets=[{}, {"pre_test": pre_test}]).run()
        pre_test.run.assert_called_once()

    def test_runs_nth_test(self):
        test = Mock()
        ClusterTestSetsRunner(sets=[{"test": test}, {}]).run()
        test.run.assert_called_once()

    def test_runs_nth_post_test(self):
        post_test = Mock()
        ClusterTestSetsRunner(sets=[{}, {"post_test": post_test}, {}]).run()
        post_test.run.assert_called_once()

    # Failure semantics

    def test_initial_failure_does_not_halt_the_set(self):
        test1 = Mock()
        test1.run.side_effect = Exception("test1 oops")
        test2 = Mock()
        with self.assertRaisesRegex(Exception, "test1 oops"):
            ClusterTestSetsRunner(sets=[{"test": test1}, {"test": test2}]).run()
        test2.run.assert_called_once()

    def test_first_failure_is_reported(self):
        test1 = Mock()
        test1.run.side_effect = Exception("test1 oops")
        test2 = Mock()
        test2.run.side_effect = Exception("test2 oops")
        with self.assertRaisesRegex(Exception, "test1 oops"):
            ClusterTestSetsRunner(sets=[{"test": test1}, {"test": test2}]).run()
