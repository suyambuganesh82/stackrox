import static org.junit.Assume.assumeTrue
import groups.BAT
import io.stackrox.proto.api.v1.SearchServiceOuterClass.RawQuery
import objects.Deployment
import objects.Job
import org.junit.experimental.categories.Category
import services.ClusterService
import services.DeploymentService
import services.ImageService
import spock.lang.Unroll
import util.Timer

class DeploymentTest extends BaseSpecification {
    private static final String DEPLOYMENT_NAME = "image-join"
    // The image name in quay.io includes the SHA from the original image
    // imported from docker.io which is somewhat confusingly different.
    private static final String DEPLOYMENT_IMAGE_NAME =
        "quay.io/rhacs-eng/qa:nginx-204a9a8e65061b10b92ad361dd6f406248404fe60efd5d6a8f2595f18bb37aad"
    private static final String DEPLOYMENT_IMAGE_SHA =
        "7413e4ab770f308c01659dd1015e61dcc1dead3923d4347dbf3c59206594332f"
    private static final String GKE_ORCHESTRATOR_DEPLOYMENT_NAME = "kube-dns"
    private static final String OPENSHIFT_ORCHESTRATOR_DEPLOYMENT_NAME = "apiserver"
    private static final String STACKROX_DEPLOYMENT_NAME = "central"

    private static final Deployment DEPLOYMENT = new Deployment()
            .setName(DEPLOYMENT_NAME)
            .setImage(DEPLOYMENT_IMAGE_NAME)
            .addLabel("app", "test")
            .setCommand(["sh", "-c", "apt-get -y update && sleep 600"])

    private static final Job JOB = new Job()
            .setName("test-job-pi")
            .setImage("quay.io/rhacs-eng/qa:perl")
            .addLabel("app", "test")
            .setCommand(["perl",  "-Mbignum=bpi", "-wle", "print bpi(2000)"])

    def setupSpec() {
        orchestrator.createDeployment(DEPLOYMENT)
    }

    def cleanupSpec() {
        orchestrator.deleteDeployment(DEPLOYMENT)
    }

    @Unroll
    @Category([BAT])
    def "Verify deployment of type Job is deleted once it completes"() {
        given:
        def job = orchestrator.createJob(JOB)

        when:
        "Make sure StackRox finds the Job"
        assert Services.waitForDeploymentByID(job.getMetadata().getUid(), JOB.name, 20, 2)

        then:
        "Wait for deletion from StackRox due to completion"
        assert Services.waitForSRDeletionByID(job.getMetadata().getUid(), JOB.name)

        cleanup:
        orchestrator.deleteJob(JOB)
    }

    @Unroll
    @Category([BAT])
    def "Verify deployment -> image links #query"() {
        when:
        Timer t = new Timer(3, 10)
        def img = null
        while (img == null && t.IsValid()) {
            img = ImageService.getImage(
                    "sha256:"+DEPLOYMENT_IMAGE_SHA, false)
        }
        assert img != null

        then:
        def results = DeploymentService.listDeploymentsSearch(RawQuery.newBuilder().setQuery(query).build())
        assert results.deploymentsList.find { x -> x.getName() == DEPLOYMENT_NAME } != null

        where:
        "Data inputs are: "
        query                                                            | _
        "Image:"+DEPLOYMENT_IMAGE_NAME                                   | _
        "Image Sha:sha256:"+DEPLOYMENT_IMAGE_SHA                         | _
        "CVE:CVE-2018-18314+Fixable:true"                                | _
        "Deployment:${DEPLOYMENT_NAME}+Image:r/quay.io.*"                | _
        "Image:r/quay.io.*"                                              | _
        "Image:!stackrox.io"                                             | _
        "Deployment:${DEPLOYMENT_NAME}+Image:!stackrox.io"               | _
        "Image Remote:rhacs-eng/qa+Image Registry:quay.io"               | _
    }

    @Unroll
    @Category([BAT])
    def "Verify image -> deployment links #query"() {
        when:
        Timer t = new Timer(3, 10)
        def img = null
        while (img == null && t.IsValid()) {
            img = ImageService.getImage(
                    "sha256:"+DEPLOYMENT_IMAGE_SHA, false)
        }
        assert img != null

        then:
        def images = ImageService.getImages(RawQuery.newBuilder().setQuery(query).build())
        assert images.find {
            x -> x.getId() == "sha256:"+DEPLOYMENT_IMAGE_SHA } != null

        where:
        "Data inputs are: "
        query                                               | _
        "Deployment:${DEPLOYMENT_NAME}"                     | _
        "Label:app=test"                                    | _
        "Image:"+DEPLOYMENT_IMAGE_NAME                      | _
        "Label:app=test+Image:"+DEPLOYMENT_IMAGE_NAME       | _
    }

    @Unroll
    @Category([BAT])
    def "Verify GKE orchestrator deployment is marked appropriately"() {
        when:
        assumeTrue(orchestrator.isGKE())

        then:
        assert checkOrchestratorDeployment(deploymentName, query, result)

        where:
        "Data inputs are: "
        deploymentName   |   query    |  result
        "${GKE_ORCHESTRATOR_DEPLOYMENT_NAME}" | "Deployment:${GKE_ORCHESTRATOR_DEPLOYMENT_NAME}+Namespace:kube-system" \
        | true
        "${STACKROX_DEPLOYMENT_NAME}"     | "Deployment:${STACKROX_DEPLOYMENT_NAME}+Namespace:stackrox" | false
    }

    @Unroll
    @Category([BAT])
    def "Verify Openshift orchestrator deployment is marked appropriately"() {
        when:
        assumeTrue(ClusterService.isOpenShift4())

        then:
        assert checkOrchestratorDeployment(deploymentName, query, result)

        where:
        "Data inputs are: "
        deploymentName   |   query    |  result
        "${OPENSHIFT_ORCHESTRATOR_DEPLOYMENT_NAME}" | "Deployment:${OPENSHIFT_ORCHESTRATOR_DEPLOYMENT_NAME}" + \
                "+Namespace:openshift-apiserver" | true
        "${STACKROX_DEPLOYMENT_NAME}"  | "Deployment:${STACKROX_DEPLOYMENT_NAME}+Namespace:stackrox" | false
    }

    boolean checkOrchestratorDeployment (String deploymentName, String query, boolean result) {
        def results = DeploymentService.listDeploymentsSearch(RawQuery.newBuilder().setQuery(query).build())
        assert results != null

        def listDep = results.deploymentsList.find { x -> x.getName() == deploymentName }
        assert listDep != null

        return DeploymentService.getDeployment(listDep.getId()).getOrchestratorComponent() == result
    }
}

