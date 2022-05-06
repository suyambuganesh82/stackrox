import io.stackrox.proto.storage.NetworkBaselineOuterClass
import io.stackrox.proto.storage.NetworkFlowOuterClass

import groups.NetworkBaseline
import objects.Deployment
import services.NetworkBaselineService
import util.NetworkGraphUtil

import org.junit.experimental.categories.Category
import spock.lang.Ignore
import spock.lang.Retry
import util.Timer

@Retry(count = 0)
class NetworkBaselineTest extends BaseSpecification {
    static final private String SERVER_DEP_NAME = "net-bl-server"
    static final private String BASELINED_CLIENT_DEP_NAME = "net-bl-client-baselined"
    static final private String ANOMALOUS_CLIENT_DEP_NAME = "net-bl-client-anomalous"
    static final private String DEFERRED_BASELINED_CLIENT_DEP_NAME = "net-bl-client-deferred-baselined"
    static final private String DEFERRED_POST_LOCK_DEP_NAME = "net-bl-client-post-lock"

    static final private String NGINX_IMAGE = "quay.io/rhacs-eng/qa:nginx-1.19-alpine"

    // The baseline generation duration must be changed from the default for this test to succeed.
    static final private int EXPECTED_BASELINE_DURATION_SECONDS = 240

    static final private int CLOCK_SKEW_ALLOWANCE_SECONDS = 15

    static final private List<Deployment> DEPLOYMENTS = []

    static final private SERVER_DEP = createAndRegisterDeployment()
                    .setName(SERVER_DEP_NAME)
                    .setImage(NGINX_IMAGE)
                    .addLabel("app", SERVER_DEP_NAME)
                    .addPort(80)
                    .setExposeAsService(true)

    static final private BASELINED_CLIENT_DEP = createAndRegisterDeployment()
                .setName(BASELINED_CLIENT_DEP_NAME)
                .setImage(NGINX_IMAGE)
                .addLabel("app", BASELINED_CLIENT_DEP_NAME)
                .setCommand(["/bin/sh", "-c",])
                .setArgs(
                    ["for i in \$(seq 1 10); do wget -S http://${SERVER_DEP_NAME}; sleep 1; done; sleep 1000" as String]
                )

    static final private ANOMALOUS_CLIENT_DEP = createAndRegisterDeployment()
        .setName(ANOMALOUS_CLIENT_DEP_NAME)
        .setImage(NGINX_IMAGE)
        .addLabel("app", ANOMALOUS_CLIENT_DEP_NAME)
        .setCommand(["/bin/sh", "-c",])
        .setArgs(["echo sleeping; date; sleep ${EXPECTED_BASELINE_DURATION_SECONDS+30}; echo sleep done; date;" +
                      "for i in \$(seq 1 10); do wget -S http://${SERVER_DEP_NAME}; sleep 1; done;" +
                      "sleep 1000" as String,])

    static final private DEFERRED_BASELINED_CLIENT_DEP = createAndRegisterDeployment()
        .setName(DEFERRED_BASELINED_CLIENT_DEP_NAME)
        .setImage(NGINX_IMAGE)
        .addLabel("app", DEFERRED_BASELINED_CLIENT_DEP_NAME)
        .setCommand(["/bin/sh", "-c",])
        .setArgs(["while sleep 1; " +
                      "do wget -S http://${SERVER_DEP_NAME}; " +
                      "done" as String,])

    static final private DEFERRED_POST_LOCK_CLIENT_DEP = createAndRegisterDeployment()
        .setName(DEFERRED_POST_LOCK_DEP_NAME)
        .setImage(NGINX_IMAGE)
        .addLabel("app", DEFERRED_POST_LOCK_DEP_NAME)
        .setCommand(["/bin/sh", "-c",])
        .setArgs(["while sleep 1; " +
                      "do wget -S http://${SERVER_DEP_NAME}; " +
                      "done" as String,])

    private static createAndRegisterDeployment() {
        Deployment deployment = new Deployment()
        DEPLOYMENTS.add(deployment)
        return deployment
    }

    private batchCreate(List<Deployment> deployments) {
        orchestrator.batchCreateDeployments(deployments)
        for (Deployment deployment : deployments) {
            assert Services.waitForDeployment(deployment)
        }
    }

    def validateBaseline(NetworkBaselineOuterClass.NetworkBaseline baseline, long beforeCreate,
                         long justAfterCreate, List<Tuple2<String, Boolean>> expectedPeers) {
        assert baseline.getObservationPeriodEnd().getSeconds() > beforeCreate - CLOCK_SKEW_ALLOWANCE_SECONDS
        assert baseline.getObservationPeriodEnd().getSeconds() <
            justAfterCreate + EXPECTED_BASELINE_DURATION_SECONDS + CLOCK_SKEW_ALLOWANCE_SECONDS
        assert baseline.getPeersCount() == expectedPeers.size()
        assert baseline.getForbiddenPeersCount() == 0

        for (def i = 0; i < expectedPeers.size(); i++) {
            def expectedPeerID = expectedPeers.get(i).getFirst()
            def expectedPeerIngress = expectedPeers.get(i).getSecond()
            def actualPeer = baseline.getPeersList().find { it.getEntity().getInfo().getId() == expectedPeerID }
            assert actualPeer
            def entityInfo = actualPeer.getEntity().getInfo()
            assert entityInfo.getType() == NetworkFlowOuterClass.NetworkEntityInfo.Type.DEPLOYMENT
            assert entityInfo.getId() == expectedPeerID
            assert actualPeer.getPropertiesCount() == 1
            def properties = actualPeer.getProperties(0)
            assert properties.getIngress() == expectedPeerIngress
            assert properties.getPort() == 80
            assert properties.getProtocol() == NetworkFlowOuterClass.L4Protocol.L4_PROTOCOL_TCP
        }
        return true
    }

    def cleanup() {
        for (Deployment deployment : DEPLOYMENTS) {
            orchestrator.deleteDeployment(deployment)
        }
    }

    @Category(NetworkBaseline)
    def "Verify network baseline functionality"() {
        when:
        "Create initial set of deployments, wait for baseline to populate"
        def beforeDeploymentCreate = System.currentTimeSeconds()
        batchCreate([SERVER_DEP, BASELINED_CLIENT_DEP])
        def justAfterDeploymentCreate = System.currentTimeSeconds()

        def serverDeploymentID = SERVER_DEP.deploymentUid
        assert serverDeploymentID != null

        def baselinedClientDeploymentID = BASELINED_CLIENT_DEP.deploymentUid
        assert baselinedClientDeploymentID != null

        assert NetworkGraphUtil.checkForEdge(baselinedClientDeploymentID, serverDeploymentID, null, 180)

        // Now create the anomalous deployment
        batchCreate([ANOMALOUS_CLIENT_DEP])

        def anomalousClientDeploymentID = ANOMALOUS_CLIENT_DEP.deploymentUid
        assert anomalousClientDeploymentID != null

        println "Deployment IDs Server: ${serverDeploymentID}, " +
                    "Baselined client: ${baselinedClientDeploymentID}, Anomalous client: ${anomalousClientDeploymentID}"

        assert NetworkGraphUtil.checkForEdge(anomalousClientDeploymentID, serverDeploymentID, null,
            EXPECTED_BASELINE_DURATION_SECONDS + 180)

        def serverBaseline = evaluateWithRetry(20, 3) {
            def baseline = NetworkBaselineService.getNetworkBaseline(serverDeploymentID)
            if (baseline.getPeersCount() == 0) {
                throw new RuntimeException(
                    "No peers in baseline for deployment ${serverDeploymentID} yet. Baseline is ${baseline}"
                )
            }
            return baseline
        }
        assert serverBaseline
        def anomalousClientBaseline = NetworkBaselineService.getNetworkBaseline(anomalousClientDeploymentID)
        assert anomalousClientBaseline
        println "Anomalous Baseline: ${anomalousClientBaseline}"
        def baselinedClientBaseline = NetworkBaselineService.getNetworkBaseline(baselinedClientDeploymentID)
        assert baselinedClientDeploymentID

        then:
        "Validate server baseline"
        // The anomalous client->server connection should not be baselined since the anonymous client
        // sleeps for a time period longer than the observation period before connecting to the server.
        validateBaseline(serverBaseline, beforeDeploymentCreate, justAfterDeploymentCreate,
            [new Tuple2<String, Boolean>(baselinedClientDeploymentID, true)])
        validateBaseline(anomalousClientBaseline, beforeDeploymentCreate, justAfterDeploymentCreate, [])
        validateBaseline(baselinedClientBaseline, beforeDeploymentCreate, justAfterDeploymentCreate,
            [new Tuple2<String, Boolean>(serverDeploymentID, false)]
        )

        when:
        "Create another deployment, ensure it gets baselined"
        def beforeDeferredCreate = System.currentTimeSeconds()
        batchCreate([DEFERRED_BASELINED_CLIENT_DEP])
        def justAfterDeferredCreate = System.currentTimeSeconds()

        def deferredBaselinedClientDeploymentID = DEFERRED_BASELINED_CLIENT_DEP.deploymentUid
        assert deferredBaselinedClientDeploymentID != null
        println "Deferred Baseline: ${deferredBaselinedClientDeploymentID}"
        // Need to chill out until the observation period ends

        sleep 180000
        println "Back from a nap"

        assert NetworkGraphUtil.checkForEdge(deferredBaselinedClientDeploymentID, serverDeploymentID, null, 180)
        serverBaseline = evaluateWithRetry(20, 3) {
            def baseline = NetworkBaselineService.getNetworkBaseline(serverDeploymentID)
            if (baseline.getPeersCount() < 2) {
                throw new RuntimeException(
                    "Not enough peers in baseline for deployment ${serverDeploymentID} yet. Baseline is ${baseline}"
                )
            }
            return baseline
        }
        assert serverBaseline

        def deferredBaselinedClientBaseline = NetworkBaselineService.getNetworkBaseline(
            deferredBaselinedClientDeploymentID
        )
        assert deferredBaselinedClientDeploymentID

        then:
        "Validate the updated baselines"
        validateBaseline(serverBaseline, beforeDeploymentCreate, justAfterDeploymentCreate,
            [new Tuple2<String, Boolean>(baselinedClientDeploymentID, true),
             // Currently, we add conns to the baseline if it's within the observation period
             // of _at least_ one of the deployments. Therefore, the deferred client->server connection
             // gets added since it's within the deferred client's obervation period, and
             // the server's baseline is modified as well since we keep things consistent.
             new Tuple2<String, Boolean>(deferredBaselinedClientDeploymentID, true),
            ]
        )
        validateBaseline(deferredBaselinedClientBaseline, beforeDeferredCreate, justAfterDeferredCreate,
            [new Tuple2<String, Boolean>(serverDeploymentID, false)])

        when:
        "Create another deployment, ensure it DOES NOT get added to serverDeploymentID due to user lock"
        NetworkBaselineService.lockNetworkBaseline(serverDeploymentID)

        def beforePostLockCreate = System.currentTimeSeconds()
        batchCreate([DEFERRED_POST_LOCK_CLIENT_DEP])
        def justAfterPostLockCreate = System.currentTimeSeconds()

        def postLockClientDeploymentID = DEFERRED_POST_LOCK_CLIENT_DEP.deploymentUid
        assert postLockClientDeploymentID != null
        println "Post Lock Deployment: ${postLockClientDeploymentID}"
        // Need to chill out until the observation period ends

        sleep 210000
        println "Back from a nap"

        assert NetworkGraphUtil.checkForEdge(postLockClientDeploymentID, serverDeploymentID, null, 180)
        serverBaseline = evaluateWithRetry(20, 3) {
            def baseline = NetworkBaselineService.getNetworkBaseline(serverDeploymentID)
            if (baseline.getPeersCount() > 2) {
                throw new RuntimeException(
                    "Too many peers for ${serverDeploymentID} due to lock. Baseline is ${baseline}"
                )
            }
            return baseline
        }
        assert serverBaseline

        print "About to do a get"
        def postLockClientBaseline = NetworkBaselineService.getNetworkBaseline(
            postLockClientDeploymentID
        )
        print "back from get, hopefully we didn't delete crap already"
        assert postLockClientBaseline
        print postLockClientBaseline

        then:
        "Validate the locked baselines"
        validateBaseline(serverBaseline, beforeDeploymentCreate, justAfterDeploymentCreate,
            [new Tuple2<String, Boolean>(baselinedClientDeploymentID, true),
             // Baseline was locked, so post lock client should not be added.
             new Tuple2<String, Boolean>(postLockClientDeploymentID, false),
            ]
        )
        validateBaseline(deferredBaselinedClientBaseline, beforeDeferredCreate, justAfterDeferredCreate,
            [new Tuple2<String, Boolean>(serverDeploymentID, false)])
    }

//     @Category(NetworkBaseline)
//     def "Verify user lock"() {
//     }
//
//     @Category(NetworkBaseline)
//     def "Verify user get for non-existent baseline"() {
//     }
}
