package resolvers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/graph-gophers/graphql-go"
	"github.com/pkg/errors"
	"github.com/stackrox/rox/central/metrics"
	"github.com/stackrox/rox/central/namespace"
	"github.com/stackrox/rox/central/node/datastore"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/logging"
	pkgMetrics "github.com/stackrox/rox/pkg/metrics"
	"github.com/stackrox/rox/pkg/search"
	"github.com/stackrox/rox/pkg/set"
	"github.com/stackrox/rox/pkg/sync"
	"github.com/stackrox/rox/pkg/utils"
)

const (
	aggregationLimit = 1000
)

var (
	log = logging.LoggerForModule()

	complianceOnce sync.Once
)

func init() {
	InitCompliance()
}

// InitCompliance is a function that registers compliance graphql resolvers with the static schema. It's exposed for
// feature flag / unit test reasons. Once the flag is gone, this can be folded into the normal init() method.
func InitCompliance() {
	complianceOnce.Do(func() {
		schema := getBuilder()
		utils.Must(
			schema.AddQuery("complianceStandard(id:ID!): ComplianceStandardMetadata"),
			schema.AddQuery("complianceStandards(query: String): [ComplianceStandardMetadata!]!"),
			schema.AddQuery("aggregatedResults(groupBy:[ComplianceAggregation_Scope!],unit:ComplianceAggregation_Scope!,where:String,collapseBy:ComplianceAggregation_Scope): ComplianceAggregation_Response!"),
			schema.AddQuery("complianceControl(id:ID!): ComplianceControl"),
			schema.AddQuery("complianceControlGroup(id:ID!): ComplianceControlGroup"),
			schema.AddQuery("complianceNamespaceCount(query: String): Int!"),
			schema.AddQuery("complianceClusterCount(query: String): Int!"),
			schema.AddQuery("complianceNodeCount(query: String): Int!"),
			schema.AddQuery("complianceDeploymentCount(query: String): Int!"),
			schema.AddQuery("executedControls(query: String): [ComplianceControlWithControlStatus!]!"),
			schema.AddQuery("executedControlCount(query: String): Int!"),
			schema.AddExtraResolver("ComplianceStandardMetadata", "controls: [ComplianceControl!]!"),
			schema.AddExtraResolver("ComplianceStandardMetadata", "groups: [ComplianceControlGroup!]!"),
			schema.AddUnionType("ComplianceDomainKey", []string{"ComplianceStandardMetadata", "ComplianceControlGroup", "ComplianceControl", "ComplianceDomain_Cluster", "ComplianceDomain_Deployment", "ComplianceDomain_Node", "Namespace"}),
			schema.AddExtraResolver("ComplianceAggregation_Result", "keys: [ComplianceDomainKey!]!"),
			schema.AddUnionType("Resource", []string{"ComplianceDomain_Deployment", "ComplianceDomain_Cluster", "ComplianceDomain_Node"}),
			schema.AddType("ControlResult", []string{"resource: Resource", "control: ComplianceControl", "value: ComplianceResultValue"}),
			schema.AddExtraResolver("ComplianceStandardMetadata", "complianceResults(query: String): [ControlResult!]!"),
			schema.AddExtraResolver("ComplianceControl", "complianceResults(query: String): [ControlResult!]!"),
			schema.AddExtraResolver("ComplianceControl", "complianceControlEntities(clusterID: ID!): [Node!]!"),
			schema.AddType("ComplianceControlNodeCount", []string{"failingCount: Int!", "passingCount: Int!", "unknownCount: Int!"}),
			schema.AddType("ComplianceControlWithControlStatus", []string{"complianceControl: ComplianceControl!", "controlStatus: String!"}),
			schema.AddExtraResolver("ComplianceControl", "complianceControlNodeCount(query: String): ComplianceControlNodeCount"),
			schema.AddExtraResolver("ComplianceControl", "complianceControlNodes(query: String): [Node!]!"),
			schema.AddExtraResolver("ComplianceControl", "complianceControlFailingNodes(query: String): [Node!]!"),
			schema.AddExtraResolver("ComplianceControl", "complianceControlPassingNodes(query: String): [Node!]!"),
		)
	})
}

// ComplianceStandards returns graphql resolvers for all compliance standards
func (resolver *Resolver) ComplianceStandards(ctx context.Context, query RawQuery) ([]*complianceStandardMetadataResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceStandards")
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	q, err := query.AsV1QueryOrEmpty()
	if err != nil {
		return nil, err
	}
	results, err := resolver.ComplianceStandardStore.SearchStandards(q)
	if err != nil {
		return nil, err
	}
	var standards []*v1.ComplianceStandardMetadata
	for _, result := range results {
		standard, ok, err := resolver.ComplianceStandardStore.Standard(result.ID)
		if !ok || err != nil {
			continue
		}
		if !resolver.manager.IsStandardActive(standard.GetMetadata().GetId()) {
			continue
		}
		standards = append(standards, standard.GetMetadata())
	}
	return resolver.wrapComplianceStandardMetadatas(standards, nil)
}

// ComplianceStandard returns a graphql resolver for a named compliance standard
func (resolver *Resolver) ComplianceStandard(ctx context.Context, args struct{ graphql.ID }) (*complianceStandardMetadataResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceStandard")
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	return resolver.wrapComplianceStandardMetadata(
		resolver.ComplianceStandardStore.StandardMetadata(string(args.ID)))
}

// ComplianceControl retrieves an individual control by ID
func (resolver *Resolver) ComplianceControl(ctx context.Context, args struct{ graphql.ID }) (*complianceControlResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceControl")
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	control := resolver.ComplianceStandardStore.Control(string(args.ID))
	return resolver.wrapComplianceControl(control, control != nil, nil)
}

// ComplianceControlGroup retrieves a control group by ID
func (resolver *Resolver) ComplianceControlGroup(ctx context.Context, args struct{ graphql.ID }) (*complianceControlGroupResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceControlGroups")
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	group := resolver.ComplianceStandardStore.Group(string(args.ID))
	return resolver.wrapComplianceControlGroup(group, group != nil, nil)
}

// ComplianceNamespaceCount returns count of namespaces that have compliance run on them
func (resolver *Resolver) ComplianceNamespaceCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceNamespaceCount")
	if err := readCompliance(ctx); err != nil {
		return 0, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_NAMESPACE}
	return resolver.getComplianceEntityCount(ctx, args, scope)
}

// ComplianceClusterCount returns count of clusters that have compliance run on them
func (resolver *Resolver) ComplianceClusterCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceClusterCount")
	if err := readCompliance(ctx); err != nil {
		return 0, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_CLUSTER}
	return resolver.getComplianceEntityCount(ctx, args, scope)
}

// ComplianceDeploymentCount returns count of deployments that have compliance run on them
func (resolver *Resolver) ComplianceDeploymentCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceDeploymentCount")
	if err := readCompliance(ctx); err != nil {
		return 0, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_DEPLOYMENT}
	return resolver.getComplianceEntityCount(ctx, args, scope)
}

// ComplianceNodeCount returns count of nodes that have compliance run on them
func (resolver *Resolver) ComplianceNodeCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ComplianceNodeCount")
	if err := readCompliance(ctx); err != nil {
		return 0, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_NODE}
	return resolver.getComplianceEntityCount(ctx, args, scope)
}

// ComplianceNamespaceCount returns count of namespaces that have compliance run on them
func (resolver *Resolver) getComplianceEntityCount(ctx context.Context, args RawQuery, scope []storage.ComplianceAggregation_Scope) (int32, error) {
	r, _, _, err := resolver.ComplianceAggregator.Aggregate(ctx, args.String(), scope, storage.ComplianceAggregation_CONTROL)
	if err != nil {
		return 0, err
	}
	return int32(len(r)), nil
}

// ExecutedControls returns the controls which have executed along with their status across clusters
func (resolver *Resolver) ExecutedControls(ctx context.Context, args RawQuery) ([]*ComplianceControlWithControlStatusResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ExecutedControls")
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_CLUSTER, storage.ComplianceAggregation_CONTROL}
	rs, _, _, err := resolver.ComplianceAggregator.Aggregate(ctx, args.String(), scope, storage.ComplianceAggregation_CONTROL)
	if err != nil {
		return nil, err
	}
	var ret []*ComplianceControlWithControlStatusResolver
	failing := make(map[string]int32)
	passing := make(map[string]int32)
	for _, r := range rs {
		controlID, err := getScopeIDFromAggregationResult(r, storage.ComplianceAggregation_CONTROL)
		if err != nil {
			return nil, err
		}
		failing[controlID] += r.GetNumFailing()
		passing[controlID] += r.GetNumPassing()
	}
	for k := range failing {
		control := resolver.ComplianceStandardStore.Control(k)
		cc := &ComplianceControlWithControlStatusResolver{
			complianceControl: &complianceControlResolver{
				root: resolver,
				data: control,
			},
		}
		cc.controlStatus = getControlStatus(failing[k], passing[k])
		ret = append(ret, cc)
	}
	return ret, nil
}

// ExecutedControlCount returns the count of controls which have executed across all clusters
func (resolver *Resolver) ExecutedControlCount(ctx context.Context, args RawQuery) (int32, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "ExecutedControls")
	if err := readCompliance(ctx); err != nil {
		return 0, err
	}
	scope := []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_CLUSTER, storage.ComplianceAggregation_CONTROL}
	rs, _, _, err := resolver.ComplianceAggregator.Aggregate(ctx, args.String(), scope, storage.ComplianceAggregation_CONTROL)
	if err != nil {
		return 0, err
	}
	controlSet := set.NewStringSet()
	for _, r := range rs {
		controlID, err := getScopeIDFromAggregationResult(r, storage.ComplianceAggregation_CONTROL)
		if err != nil {
			return 0, err
		}
		controlSet.Add(controlID)
	}
	return int32(controlSet.Cardinality()), nil
}

type aggregatedResultQuery struct {
	GroupBy    *[]string
	Unit       string
	Where      *string
	CollapseBy *string
}

// AggregatedResults returns the aggregation of the last runs aggregated by scope, unit and filtered by a query
func (resolver *Resolver) AggregatedResults(ctx context.Context, args aggregatedResultQuery) (*complianceAggregationResponseWithDomainResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Root, "AggregatedResults")

	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	var where string
	if args.Where != nil {
		where = *args.Where
	}

	groupBy := toComplianceAggregation_Scopes(args.GroupBy)
	unit := toComplianceAggregation_Scope(&args.Unit)
	collapseBy := toComplianceAggregation_Scope(args.CollapseBy)

	validResults, sources, domainMap, err := resolver.ComplianceAggregator.Aggregate(ctx, where, groupBy, unit)
	if err != nil {
		return nil, err
	}

	validResults, domainMap, errMsg := truncateResults(validResults, domainMap, collapseBy)

	return &complianceAggregationResponseWithDomainResolver{
		complianceAggregation_ResponseResolver: complianceAggregation_ResponseResolver{
			root: resolver,
			data: &storage.ComplianceAggregation_Response{
				Results:      validResults,
				Sources:      sources,
				ErrorMessage: errMsg,
			},
		},
		domainMap: domainMap,
	}, nil
}

// "\nschema {\n\tquery: Query\n\tmutation: Mutation\n}\n\ntype Query {\n\taggregatedResults(groupBy:[ComplianceAggregation_Scope!],unit:ComplianceAggregation_Scope!,where:String,collapseBy:ComplianceAggregation_Scope): ComplianceAggregation_Response!\n\talertComments(resourceId: ID!): [Comment!]!\n\tcluster(id: ID!): Cluster\n\tclusterCount(query: String): Int!\n\tclusterHealthCounter(query: String): ClusterHealthCounter!\n\tclusters(query: String, pagination: Pagination): [Cluster!]!\n\tcomplianceClusterCount(query: String): Int!\n\tcomplianceControl(id:ID!): ComplianceControl\n\tcomplianceControlGroup(id:ID!): ComplianceControlGroup\n\tcomplianceDeploymentCount(query: String): Int!\n\tcomplianceNamespaceCount(query: String): Int!\n\tcomplianceNodeCount(query: String): Int!\n\tcomplianceRecentRuns(clusterId:ID, standardId:ID, since:Time): [ComplianceRun!]!\n\tcomplianceRun(id:ID!): ComplianceRun\n\tcomplianceRunStatuses(ids: [ID!]!): GetComplianceRunStatusesResponse!\n\tcomplianceStandard(id:ID!): ComplianceStandardMetadata\n\tcomplianceStandards(query: String): [ComplianceStandardMetadata!]!\n\tcomponent(id: ID): EmbeddedImageScanComponent\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tdeployment(id: ID): Deployment\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tdeploymentsWithMostSevereViolations(query: String, pagination: Pagination): [DeploymentsWithMostSevereViolations!]!\n\texecutedControlCount(query: String): Int!\n\texecutedControls(query: String): [ComplianceControlWithControlStatus!]!\n\tglobalSearch(categories: [SearchCategory!], query: String!): [SearchResult!]!\n\tgroup(authProviderId: String, key: String, value: String): Group\n\tgroupedContainerInstances(query: String): [ContainerNameGroup!]!\n\tgroups: [Group!]!\n\timage(id: ID!): Image\n\timageCount(query: String): Int!\n\timages(query: String, pagination: Pagination): [Image!]!\n\tistioVulnerabilities(query: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tistioVulnerability(id: ID): EmbeddedVulnerability\n\tk8sRole(id: ID!): K8SRole\n\tk8sRoleCount(query: String): Int!\n\tk8sRoles(query: String, pagination: Pagination): [K8SRole!]!\n\tk8sVulnerabilities(query: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tk8sVulnerability(id: ID): EmbeddedVulnerability\n\tmyPermissions: GetPermissionsResponse\n\tnamespace(id: ID!): Namespace\n\tnamespaceByClusterIDAndName(clusterID: ID!, name: String!): Namespace\n\tnamespaceCount(query: String): Int!\n\tnamespaces(query: String, pagination: Pagination): [Namespace!]!\n\tnode(id:ID!): Node\n\tnodeCount(query: String): Int!\n\tnodes(query: String, pagination: Pagination): [Node!]!\n\tnotifier(id: ID!): Notifier\n\tnotifiers: [Notifier!]!\n\tpermissionSet(id: ID): PermissionSet\n\tpermissionSets: [PermissionSet!]!\n\tpod(id: ID): Pod\n\tpodCount(query: String): Int!\n\tpods(query: String, pagination: Pagination): [Pod!]!\n\tpolicies(query: String, pagination: Pagination): [Policy!]!\n\tpolicy(id: ID): Policy\n\tpolicyCount(query: String): Int!\n\tprocessComments(key: ProcessNoteKey!): [Comment!]!\n\tprocessCommentsCount(key: ProcessNoteKey!): Int!\n\tprocessTags(key: ProcessNoteKey!): [String!]!\n\tprocessTagsCount(key: ProcessNoteKey!): Int!\n\trole(id: ID): Role\n\troles: [Role!]!\n\tsearchAutocomplete(categories: [SearchCategory!], query: String!): [String!]!\n\tsearchOptions(categories: [SearchCategory!]): [String!]!\n\tsecret(id:ID!): Secret\n\tsecretCount(query: String): Int!\n\tsecrets(query: String, pagination: Pagination): [Secret!]!\n\tserviceAccount(id: ID!): ServiceAccount\n\tserviceAccountCount(query: String): Int!\n\tserviceAccounts(query: String, pagination: Pagination): [ServiceAccount!]!\n\tsimpleAccessScope(id: ID): SimpleAccessScope\n\tsimpleAccessScopes: [SimpleAccessScope!]!\n\tsubject(id: ID): Subject\n\tsubjectCount(query: String): Int!\n\tsubjects(query: String, pagination: Pagination): [Subject!]!\n\ttoken(id:ID!): TokenMetadata\n\ttokens(revoked:Boolean): [TokenMetadata!]!\n\tviolation(id: ID!): Alert\n\tviolationCount(query: String): Int!\n\tviolations(query: String, pagination: Pagination): [Alert!]!\n\tvulnerabilities(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tvulnerability(id: ID): EmbeddedVulnerability\n\tvulnerabilityCount(query: String): Int!\n\tvulnerabilityRequest(id: ID!): VulnerabilityRequest\n\tvulnerabilityRequests(query: String, requestIDSelector: String, pagination: Pagination): [VulnerabilityRequest!]!\n\tvulnerabilityRequestsCount(query: String): Int!\n}\n\ntype Mutation {\n\taddAlertComment(resourceId: ID!, commentMessage: String!): String!\n\taddAlertTags(resourceId: ID!, tags: [String!]!): [String!]!\n\taddProcessComment(key: ProcessNoteKey!, commentMessage: String!): String!\n\taddProcessTags(key: ProcessNoteKey!, tags: [String!]!): Boolean!\n\tapproveVulnerabilityRequest(requestID: ID!, comment: String!): VulnerabilityRequest!\n\tbulkAddAlertTags(resourceIds: [ID!]!, tags: [String!]!): [String!]!\n\tcomplianceTriggerRuns(clusterId:ID!,standardId:ID!): [ComplianceRun!]!\n\tdeferVulnerability(request: DeferVulnRequest!): VulnerabilityRequest!\n\tdeleteVulnerabilityRequest(requestID: ID!): Boolean!\n\tdenyVulnerabilityRequest(requestID: ID!, comment: String!): VulnerabilityRequest!\n\tmarkVulnerabilityFalsePositive(request: FalsePositiveVulnRequest!): VulnerabilityRequest!\n\tremoveAlertComment(resourceId: ID!, commentId: ID!): Boolean!\n\tremoveAlertTags(resourceId: ID!, tags: [String!]!): Boolean!\n\tremoveProcessComment(key: ProcessNoteKey!, commentId: ID!): Boolean!\n\tremoveProcessTags(key: ProcessNoteKey!, tags: [String!]!): Boolean!\n\tundoVulnerabilityRequest(requestID: ID!): VulnerabilityRequest!\n\tupdateAlertComment(resourceId: ID!, commentId: ID!, commentMessage: String!): Boolean!\n\tupdateProcessComment(key: ProcessNoteKey!, commentId: ID!, commentMessage: String!): Boolean!\n\tupdateVulnerabilityRequest(requestID: ID!, comment: String!, expiry: VulnReqExpiry!): VulnerabilityRequest!\n}\n\ntype AWSProviderMetadata {\n\taccountId: String!\n}\n\ntype AWSSecurityHub {\n\taccountId: String!\n\tcredentials: AWSSecurityHub_Credentials\n\tregion: String!\n}\n\ntype AWSSecurityHub_Credentials {\n\taccessKeyId: String!\n\tsecretAccessKey: String!\n}\n\n\nenum Access {\n    NO_ACCESS\n    READ_ACCESS\n    READ_WRITE_ACCESS\n}\n\ntype ActiveComponent_ActiveContext {\n\tcontainerName: String!\n\timageId: String!\n}\n\ntype ActiveState {\n\tstate: String!\n\tactiveContexts: [ActiveComponent_ActiveContext!]!\n}\n\ntype AdmissionControlHealthInfo {\n\tstatusErrors: [String!]!\n}\n\ntype AdmissionControllerConfig {\n\tdisableBypass: Boolean!\n\tenabled: Boolean!\n\tenforceOnUpdates: Boolean!\n\tscanInline: Boolean!\n\ttimeoutSeconds: Int!\n}\n\ntype Alert {\n\tclusterId: String!\n\tclusterName: String!\n\tdeployment: Alert_Deployment\n\tenforcement: Alert_Enforcement\n\tfirstOccurred: Time\n\tid: ID!\n\timage: ContainerImage\n\tlifecycleStage: LifecycleStage!\n\tnamespace: String!\n\tnamespaceId: String!\n\tpolicy: Policy\n\tprocessViolation: Alert_ProcessViolation\n\tresolvedAt: Time\n\tresource: Alert_Resource\n\tsnoozeTill: Time\n\tstate: ViolationState!\n\ttags: [String!]!\n\ttime: Time\n\tviolations: [Alert_Violation]!\n\tentity: AlertEntity\n\tunusedVarSink(query: String): Int\n}\n\nunion AlertEntity = Alert_Deployment | ContainerImage | Alert_Resource\n\ntype Alert_Deployment {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tcontainers: [Alert_Deployment_Container]!\n\tid: ID!\n\tinactive: Boolean!\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\tnamespaceId: String!\n\ttype: String!\n}\n\ntype Alert_Deployment_Container {\n\timage: ContainerImage\n\tname: String!\n}\n\ntype Alert_Enforcement {\n\taction: EnforcementAction!\n\tmessage: String!\n}\n\ntype Alert_ProcessViolation {\n\tmessage: String!\n\tprocesses: [ProcessIndicator]!\n}\n\ntype Alert_Resource {\n\tclusterId: String!\n\tclusterName: String!\n\tname: String!\n\tnamespace: String!\n\tnamespaceId: String!\n\tresourceType: Alert_Resource_ResourceType!\n}\n\n\nenum Alert_Resource_ResourceType {\n    UNKNOWN\n    SECRETS\n    CONFIGMAPS\n}\n\ntype Alert_Violation {\n\tkeyValueAttrs: Alert_Violation_KeyValueAttrs\n\tmessage: String!\n\tnetworkFlowInfo: Alert_Violation_NetworkFlowInfo\n\ttime: Time\n\ttype: Alert_Violation_Type!\n\tmessageAttributes: Alert_ViolationMessageAttributes\n}\n\nunion Alert_ViolationMessageAttributes = Alert_Violation_KeyValueAttrs | Alert_Violation_NetworkFlowInfo\n\ntype Alert_Violation_KeyValueAttrs {\n\tattrs: [Alert_Violation_KeyValueAttrs_KeyValueAttr]!\n}\n\ntype Alert_Violation_KeyValueAttrs_KeyValueAttr {\n\tkey: String!\n\tvalue: String!\n}\n\ntype Alert_Violation_NetworkFlowInfo {\n\tdestination: Alert_Violation_NetworkFlowInfo_Entity\n\tprotocol: L4Protocol!\n\tsource: Alert_Violation_NetworkFlowInfo_Entity\n}\n\ntype Alert_Violation_NetworkFlowInfo_Entity {\n\tdeploymentNamespace: String!\n\tdeploymentType: String!\n\tentityType: NetworkEntityInfo_Type!\n\tname: String!\n\tport: Int!\n}\n\n\nenum Alert_Violation_Type {\n    GENERIC\n    K8S_EVENT\n    NETWORK_FLOW\n}\n\ntype AzureProviderMetadata {\n\tsubscriptionId: String!\n}\n\n\nenum BooleanOperator {\n    OR\n    AND\n}\n\ntype CSCC {\n\tserviceAccount: String!\n\tsourceId: String!\n}\n\ntype CVE {\n\tcreatedAt: Time\n\tid: ID!\n\timpactScore: Float!\n\tlastModified: Time\n\tlink: String!\n\toperatingSystem: String!\n\tpublishedOn: Time\n\treferences: [CVE_Reference]!\n\tscoreVersion: CVE_ScoreVersion!\n\tseverity: VulnerabilitySeverity!\n\tsummary: String!\n\tsuppressActivation: Time\n\tsuppressExpiry: Time\n\tsuppressed: Boolean!\n\ttype: CVE_CVEType!\n\ttypes: [CVE_CVEType!]!\n}\n\n\nenum CVE_CVEType {\n    UNKNOWN_CVE\n    IMAGE_CVE\n    K8S_CVE\n    ISTIO_CVE\n    NODE_CVE\n    OPENSHIFT_CVE\n}\n\ntype CVE_Reference {\n\ttags: [String!]!\n\tuRI: String!\n}\n\n\nenum CVE_ScoreVersion {\n    V2\n    V3\n    UNKNOWN\n}\n\ntype CVSSV2 {\n\taccessComplexity: CVSSV2_AccessComplexity!\n\tattackVector: CVSSV2_AttackVector!\n\tauthentication: CVSSV2_Authentication!\n\tavailability: CVSSV2_Impact!\n\tconfidentiality: CVSSV2_Impact!\n\texploitabilityScore: Float!\n\timpactScore: Float!\n\tintegrity: CVSSV2_Impact!\n\tscore: Float!\n\tseverity: CVSSV2_Severity!\n\tvector: String!\n}\n\n\nenum CVSSV2_AccessComplexity {\n    ACCESS_HIGH\n    ACCESS_MEDIUM\n    ACCESS_LOW\n}\n\n\nenum CVSSV2_AttackVector {\n    ATTACK_LOCAL\n    ATTACK_ADJACENT\n    ATTACK_NETWORK\n}\n\n\nenum CVSSV2_Authentication {\n    AUTH_MULTIPLE\n    AUTH_SINGLE\n    AUTH_NONE\n}\n\n\nenum CVSSV2_Impact {\n    IMPACT_NONE\n    IMPACT_PARTIAL\n    IMPACT_COMPLETE\n}\n\n\nenum CVSSV2_Severity {\n    UNKNOWN\n    LOW\n    MEDIUM\n    HIGH\n}\n\ntype CVSSV3 {\n\tattackComplexity: CVSSV3_Complexity!\n\tattackVector: CVSSV3_AttackVector!\n\tavailability: CVSSV3_Impact!\n\tconfidentiality: CVSSV3_Impact!\n\texploitabilityScore: Float!\n\timpactScore: Float!\n\tintegrity: CVSSV3_Impact!\n\tprivilegesRequired: CVSSV3_Privileges!\n\tscope: CVSSV3_Scope!\n\tscore: Float!\n\tseverity: CVSSV3_Severity!\n\tuserInteraction: CVSSV3_UserInteraction!\n\tvector: String!\n}\n\n\nenum CVSSV3_AttackVector {\n    ATTACK_LOCAL\n    ATTACK_ADJACENT\n    ATTACK_NETWORK\n    ATTACK_PHYSICAL\n}\n\n\nenum CVSSV3_Complexity {\n    COMPLEXITY_LOW\n    COMPLEXITY_HIGH\n}\n\n\nenum CVSSV3_Impact {\n    IMPACT_NONE\n    IMPACT_LOW\n    IMPACT_HIGH\n}\n\n\nenum CVSSV3_Privileges {\n    PRIVILEGE_NONE\n    PRIVILEGE_LOW\n    PRIVILEGE_HIGH\n}\n\n\nenum CVSSV3_Scope {\n    UNCHANGED\n    CHANGED\n}\n\n\nenum CVSSV3_Severity {\n    UNKNOWN\n    NONE\n    LOW\n    MEDIUM\n    HIGH\n    CRITICAL\n}\n\n\nenum CVSSV3_UserInteraction {\n    UI_NONE\n    UI_REQUIRED\n}\n\ntype Cert {\n\talgorithm: String!\n\tendDate: Time\n\tissuer: CertName\n\tsans: [String!]!\n\tstartDate: Time\n\tsubject: CertName\n}\n\ntype CertName {\n\tcommonName: String!\n\tcountry: String!\n\tlocality: String!\n\tnames: [String!]!\n\torganization: String!\n\torganizationUnit: String!\n\tpostalCode: String!\n\tprovince: String!\n\tstreetAddress: String!\n}\n\ntype Cluster {\n\tadmissionController: Boolean!\n\tadmissionControllerEvents: Boolean!\n\tadmissionControllerUpdates: Boolean!\n\tcentralApiEndpoint: String!\n\tcollectionMethod: CollectionMethod!\n\tcollectorImage: String!\n\tdynamicConfig: DynamicClusterConfig\n\thealthStatus: ClusterHealthStatus\n\thelmConfig: CompleteClusterConfig\n\tid: ID!\n\tinitBundleId: String!\n\tlabels: [Label!]!\n\tmainImage: String!\n\tmanagedBy: ManagerType!\n\tmostRecentSensorId: SensorDeploymentIdentification\n\tname: String!\n\tpriority: Int!\n\truntimeSupport: Boolean!\n\tslimCollector: Boolean!\n\tstatus: ClusterStatus\n\ttolerationsConfig: TolerationsConfig\n\ttype: ClusterType!\n\talertCount(query: String): Int!\n\talerts(query: String, pagination: Pagination): [Alert!]!\n\tcomplianceControlCount(query: String): ComplianceControlCount!\n\tcomplianceResults(query: String): [ControlResult!]!\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tcontrolStatus(query: String): String!\n\tcontrols(query: String): [ComplianceControl!]!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tfailingControls(query: String): [ComplianceControl!]!\n\tfailingPolicyCounter(query: String): PolicyCounter\n\timageCount(query: String): Int!\n\timages(query: String, pagination: Pagination): [Image!]!\n\tisGKECluster: Boolean!\n\tisOpenShiftCluster: Boolean!\n\tistioEnabled: Boolean!\n\tistioVulnCount(query: String): Int!\n\tistioVulns(query: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tk8sRole(role: ID!): K8SRole\n\tk8sRoleCount(query: String): Int!\n\tk8sRoles(query: String, pagination: Pagination): [K8SRole!]!\n\tk8sVulnCount(query: String): Int!\n\tk8sVulns(query: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tlatestViolation(query: String): Time\n\tnamespace(name: String!): Namespace\n\tnamespaceCount(query: String): Int!\n\tnamespaces(query: String, pagination: Pagination): [Namespace!]!\n\tnode(node: ID!): Node\n\tnodeCount(query: String): Int!\n\tnodes(query: String, pagination: Pagination): [Node!]!\n\topenShiftVulnCount(query: String): Int!\n\topenShiftVulns(query: String, pagination: Pagination): [EmbeddedVulnerability!]!\n\tpassingControls(query: String): [ComplianceControl!]!\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\tpolicies(query: String, pagination: Pagination): [Policy!]!\n\tpolicyCount(query: String): Int!\n\tpolicyStatus(query: String): PolicyStatus!\n\trisk: Risk\n\tsecretCount(query: String): Int!\n\tsecrets(query: String, pagination: Pagination): [Secret!]!\n\tserviceAccount(sa: ID!): ServiceAccount\n\tserviceAccountCount(query: String): Int!\n\tserviceAccounts(query: String, pagination: Pagination): [ServiceAccount!]!\n\tsubject(name: String!): Subject\n\tsubjectCount(query: String): Int!\n\tsubjects(query: String, pagination: Pagination): [Subject!]!\n\tunusedVarSink(query: String): Int\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability!]!\n}\n\ntype ClusterCertExpiryStatus {\n\tsensorCertExpiry: Time\n\tsensorCertNotBefore: Time\n}\n\ntype ClusterHealthCounter {\n\ttotal: Int!\n\tuninitialized: Int!\n\thealthy: Int!\n\tdegraded: Int!\n\tunhealthy: Int!\n}\n\ntype ClusterHealthStatus {\n\tadmissionControlHealthInfo: AdmissionControlHealthInfo\n\tadmissionControlHealthStatus: ClusterHealthStatus_HealthStatusLabel!\n\tcollectorHealthInfo: CollectorHealthInfo\n\tcollectorHealthStatus: ClusterHealthStatus_HealthStatusLabel!\n\thealthInfoComplete: Boolean!\n\tid: ID!\n\tlastContact: Time\n\toverallHealthStatus: ClusterHealthStatus_HealthStatusLabel!\n\tsensorHealthStatus: ClusterHealthStatus_HealthStatusLabel!\n}\n\n\nenum ClusterHealthStatus_HealthStatusLabel {\n    UNINITIALIZED\n    UNAVAILABLE\n    UNHEALTHY\n    DEGRADED\n    HEALTHY\n}\n\ntype ClusterStatus {\n\tcertExpiryStatus: ClusterCertExpiryStatus\n\torchestratorMetadata: OrchestratorMetadata\n\tproviderMetadata: ProviderMetadata\n\tsensorVersion: String!\n\tupgradeStatus: ClusterUpgradeStatus\n}\n\n\nenum ClusterType {\n    GENERIC_CLUSTER\n    KUBERNETES_CLUSTER\n    OPENSHIFT_CLUSTER\n    OPENSHIFT4_CLUSTER\n}\n\ntype ClusterUpgradeStatus {\n\tmostRecentProcess: ClusterUpgradeStatus_UpgradeProcessStatus\n\tupgradability: ClusterUpgradeStatus_Upgradability!\n\tupgradabilityStatusReason: String!\n}\n\n\nenum ClusterUpgradeStatus_Upgradability {\n    UNSET\n    UP_TO_DATE\n    MANUAL_UPGRADE_REQUIRED\n    AUTO_UPGRADE_POSSIBLE\n    SENSOR_VERSION_HIGHER\n}\n\ntype ClusterUpgradeStatus_UpgradeProcessStatus {\n\tactive: Boolean!\n\tid: ID!\n\tinitiatedAt: Time\n\tprogress: UpgradeProgress\n\ttargetVersion: String!\n\ttype: ClusterUpgradeStatus_UpgradeProcessStatus_UpgradeProcessType!\n\tupgraderImage: String!\n}\n\n\nenum ClusterUpgradeStatus_UpgradeProcessStatus_UpgradeProcessType {\n    UPGRADE\n    CERT_ROTATION\n}\n\n\nenum CollectionMethod {\n    UNSET_COLLECTION\n    NO_COLLECTION\n    KERNEL_MODULE\n    EBPF\n}\n\ntype CollectorHealthInfo {\n\tstatusErrors: [String!]!\n\tversion: String!\n}\n\ntype Comment {\n\tcommentId: String!\n\tcommentMessage: String!\n\tcreatedAt: Time\n\tlastModified: Time\n\tresourceId: String!\n\tresourceType: ResourceType!\n\tuser: Comment_User\n\tdeletable: Boolean!\n\tmodifiable: Boolean!\n}\n\ntype Comment_User {\n\temail: String!\n\tid: ID!\n\tname: String!\n}\n\n\nenum Comparator {\n    LESS_THAN\n    LESS_THAN_OR_EQUALS\n    EQUALS\n    GREATER_THAN_OR_EQUALS\n    GREATER_THAN\n}\n\ntype CompleteClusterConfig {\n\tclusterLabels: [Label!]!\n\tconfigFingerprint: String!\n\tdynamicConfig: DynamicClusterConfig\n\tstaticConfig: StaticClusterConfig\n}\n\ntype ComplianceAggregation_AggregationKey {\n\tid: ID!\n\tscope: ComplianceAggregation_Scope!\n}\n\ntype ComplianceAggregation_Response {\n\terrorMessage: String!\n\tresults: [ComplianceAggregation_Result]!\n\tsources: [ComplianceAggregation_Source]!\n}\n\ntype ComplianceAggregation_Result {\n\taggregationKeys: [ComplianceAggregation_AggregationKey]!\n\tnumFailing: Int!\n\tnumPassing: Int!\n\tnumSkipped: Int!\n\tunit: ComplianceAggregation_Scope!\n\tkeys: [ComplianceDomainKey!]!\n}\n\n\nenum ComplianceAggregation_Scope {\n    UNKNOWN\n    STANDARD\n    CLUSTER\n    CATEGORY\n    CONTROL\n    NAMESPACE\n    NODE\n    DEPLOYMENT\n    CHECK\n}\n\ntype ComplianceAggregation_Source {\n\tclusterId: String!\n\tfailedRuns: [ComplianceRunMetadata]!\n\tstandardId: String!\n\tsuccessfulRun: ComplianceRunMetadata\n}\n\ntype ComplianceControl {\n\tdescription: String!\n\tgroupId: String!\n\tid: ID!\n\timplemented: Boolean!\n\tinterpretationText: String!\n\tname: String!\n\tstandardId: String!\n\tcomplianceControlEntities(clusterID: ID!): [Node!]!\n\tcomplianceControlFailingNodes(query: String): [Node!]!\n\tcomplianceControlNodeCount(query: String): ComplianceControlNodeCount\n\tcomplianceControlNodes(query: String): [Node!]!\n\tcomplianceControlPassingNodes(query: String): [Node!]!\n\tcomplianceResults(query: String): [ControlResult!]!\n}\n\ntype ComplianceControlCount {\n\tfailingCount: Int!\n\tpassingCount: Int!\n\tunknownCount: Int!\n}\n\ntype ComplianceControlGroup {\n\tdescription: String!\n\tid: ID!\n\tname: String!\n\tnumImplementedChecks: Int!\n\tstandardId: String!\n}\n\ntype ComplianceControlNodeCount {\n\tfailingCount: Int!\n\tpassingCount: Int!\n\tunknownCount: Int!\n}\n\ntype ComplianceControlResult {\n\tcontrolId: String!\n\tresource: ComplianceResource\n\tvalue: ComplianceResultValue\n}\n\ntype ComplianceControlWithControlStatus {\n\tcomplianceControl: ComplianceControl!\n\tcontrolStatus: String!\n}\n\nunion ComplianceDomainKey = ComplianceStandardMetadata | ComplianceControlGroup | ComplianceControl | Cluster | Deployment | Node | Namespace\n\ntype ComplianceDomain_Cluster {\n\tid: ID!\n\tname: String!\n}\n\ntype ComplianceDomain_Deployment {\n\tclusterName: String!\n\tid: ID!\n\tname: String!\n\tnamespace: String!\n\tnamespaceId: String!\n\ttype: String!\n}\n\ntype ComplianceDomain_Node {\n\tclusterName: String!\n\tid: ID!\n\tname: String!\n}\n\ntype ComplianceResource {\n\tcluster: ComplianceResource_ClusterName\n\tdeployment: ComplianceResource_DeploymentName\n\timage: ImageName\n\tnode: ComplianceResource_NodeName\n\tresource: ComplianceResourceResource\n}\n\nunion ComplianceResourceResource = ComplianceResource_ClusterName | ComplianceResource_DeploymentName | ComplianceResource_NodeName | ImageName\n\ntype ComplianceResource_ClusterName {\n\tid: ID!\n\tname: String!\n}\n\ntype ComplianceResource_DeploymentName {\n\tcluster: ComplianceResource_ClusterName\n\tid: ID!\n\tname: String!\n\tnamespace: String!\n}\n\ntype ComplianceResource_NodeName {\n\tcluster: ComplianceResource_ClusterName\n\tid: ID!\n\tname: String!\n}\n\ntype ComplianceResultValue {\n\tevidence: [ComplianceResultValue_Evidence]!\n\toverallState: ComplianceState!\n}\n\ntype ComplianceResultValue_Evidence {\n\tmessage: String!\n\tmessageId: Int!\n\tstate: ComplianceState!\n}\n\ntype ComplianceRun {\n\tclusterId: String!\n\terrorMessage: String!\n\tfinishTime: Time\n\tid: ID!\n\tscheduleId: String!\n\tstandardId: String!\n\tstartTime: Time\n\tstate: ComplianceRun_State!\n}\n\ntype ComplianceRunMetadata {\n\tclusterId: String!\n\tdomainId: String!\n\terrorMessage: String!\n\tfinishTimestamp: Time\n\trunId: String!\n\tstandardId: String!\n\tstartTimestamp: Time\n\tsuccess: Boolean!\n}\n\ntype ComplianceRunSchedule {\n\tclusterId: String!\n\tcrontabSpec: String!\n\tid: ID!\n\tstandardId: String!\n\tsuspended: Boolean!\n}\n\ntype ComplianceRunScheduleInfo {\n\tlastCompletedRun: ComplianceRun\n\tlastRun: ComplianceRun\n\tnextRunTime: Time\n\tschedule: ComplianceRunSchedule\n}\n\n\nenum ComplianceRun_State {\n    INVALID\n    READY\n    STARTED\n    WAIT_FOR_DATA\n    EVALUTING_CHECKS\n    FINISHED\n}\n\ntype ComplianceStandard {\n\tcontrols: [ComplianceControl]!\n\tgroups: [ComplianceControlGroup]!\n\tmetadata: ComplianceStandardMetadata\n}\n\ntype ComplianceStandardMetadata {\n\tdescription: String!\n\tdynamic: Boolean!\n\tid: ID!\n\tname: String!\n\tnumImplementedChecks: Int!\n\tscopes: [ComplianceStandardMetadata_Scope!]!\n\tcomplianceResults(query: String): [ControlResult!]!\n\tcontrols: [ComplianceControl!]!\n\tgroups: [ComplianceControlGroup!]!\n}\n\n\nenum ComplianceStandardMetadata_Scope {\n    UNSET\n    CLUSTER\n    NAMESPACE\n    DEPLOYMENT\n    NODE\n}\n\n\nenum ComplianceState {\n    COMPLIANCE_STATE_UNKNOWN\n    COMPLIANCE_STATE_SKIP\n    COMPLIANCE_STATE_NOTE\n    COMPLIANCE_STATE_SUCCESS\n    COMPLIANCE_STATE_FAILURE\n    COMPLIANCE_STATE_ERROR\n}\n\ntype Component {\n\tname: String!\n\tversion: String!\n}\n\ntype Container {\n\tconfig: ContainerConfig\n\tid: ID!\n\timage: ContainerImage\n\tlivenessProbe: LivenessProbe\n\tname: String!\n\tports: [PortConfig]!\n\treadinessProbe: ReadinessProbe\n\tresources: Resources\n\tsecrets: [EmbeddedSecret]!\n\tsecurityContext: SecurityContext\n\tvolumes: [Volume]!\n}\n\ntype ContainerConfig {\n\tappArmorProfile: String!\n\targs: [String!]!\n\tcommand: [String!]!\n\tdirectory: String!\n\tenv: [ContainerConfig_EnvironmentConfig]!\n\tuid: Int!\n\tuser: String!\n}\n\ntype ContainerConfig_EnvironmentConfig {\n\tenvVarSource: ContainerConfig_EnvironmentConfig_EnvVarSource!\n\tkey: String!\n\tvalue: String!\n}\n\n\nenum ContainerConfig_EnvironmentConfig_EnvVarSource {\n    UNSET\n    RAW\n    SECRET_KEY\n    CONFIG_MAP_KEY\n    FIELD\n    RESOURCE_FIELD\n    UNKNOWN\n}\n\ntype ContainerImage {\n\tid: ID!\n\tisClusterLocal: Boolean!\n\tname: ImageName\n\tnotPullable: Boolean!\n}\n\ntype ContainerInstance {\n\tcontainerIps: [String!]!\n\tcontainerName: String!\n\tcontainingPodId: String!\n\texitCode: Int!\n\tfinished: Time\n\timageDigest: String!\n\tinstanceId: ContainerInstanceID\n\tstarted: Time\n\tterminationReason: String!\n}\n\ntype ContainerInstanceID {\n\tcontainerRuntime: ContainerRuntime!\n\tid: ID!\n\tnode: String!\n}\n\ntype ContainerNameGroup {\n\tid: ID!\n\tname: String!\n\tpodId: String!\n\tstartTime: Time\n\tcontainerInstances: [ContainerInstance!]!\n\tevents: [DeploymentEvent!]!\n}\n\ntype ContainerRestartEvent implements DeploymentEvent {\n\tid: ID!\n\tname: String!\n\ttimestamp: Time\n}\n\n\nenum ContainerRuntime {\n    UNKNOWN_CONTAINER_RUNTIME\n    DOCKER_CONTAINER_RUNTIME\n    CRIO_CONTAINER_RUNTIME\n}\n\ntype ContainerRuntimeInfo {\n\ttype: ContainerRuntime!\n\tversion: String!\n}\n\ntype ContainerTerminationEvent implements DeploymentEvent {\n\tid: ID!\n\tname: String!\n\ttimestamp: Time\n\texitCode: Int!\n\treason: String!\n}\n\ntype ControlResult {\n\tresource: Resource\n\tcontrol: ComplianceControl\n\tvalue: ComplianceResultValue\n}\n\ntype CosignSignature {\n}\n\ntype DataSource {\n\tid: ID!\n\tname: String!\n}\n\ninput DeferVulnRequest {\n\tcomment: String\n\tcve: String\n\texpiresOn: Time\n\texpiresWhenFixed: Boolean\n\tscope: VulnReqScope\n}\n\ntype DeferralRequest {\n\texpiresOn: Time\n\texpiresWhenFixed: Boolean!\n}\n\ntype Deployment {\n\tannotations: [Label!]!\n\tautomountServiceAccountToken: Boolean!\n\tclusterId: String!\n\tclusterName: String!\n\tcontainers: [Container]!\n\tcreated: Time\n\thostIpc: Boolean!\n\thostNetwork: Boolean!\n\thostPid: Boolean!\n\tid: ID!\n\timagePullSecrets: [String!]!\n\tinactive: Boolean!\n\tlabelSelector: LabelSelector\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\tnamespaceId: String!\n\torchestratorComponent: Boolean!\n\tpodLabels: [Label!]!\n\tports: [PortConfig]!\n\tpriority: Int!\n\tprocessTags: [String!]!\n\treplicas: Int!\n\triskScore: Float!\n\truntimeClass: String!\n\tserviceAccount: String!\n\tserviceAccountPermissionLevel: PermissionLevel!\n\tstateTimestamp: Int!\n\ttolerations: [Toleration]!\n\ttype: String!\n\tcluster: Cluster\n\tcomplianceResults(query: String): [ControlResult!]!\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tcontainerRestartCount: Int!\n\tcontainerTerminationCount: Int!\n\tdeployAlertCount(query: String): Int!\n\tdeployAlerts(query: String, pagination: Pagination): [Alert!]!\n\tfailingPolicies(query: String, pagination: Pagination): [Policy!]!\n\tfailingPolicyCount(query: String): Int!\n\tfailingPolicyCounter(query: String): PolicyCounter\n\tfailingRuntimePolicyCount(query: String): Int!\n\tgroupedProcesses: [ProcessNameGroup!]!\n\timageCount(query: String): Int!\n\timages(query: String, pagination: Pagination): [Image!]!\n\tlatestViolation(query: String): Time\n\tnamespaceObject: Namespace\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\tpodCount: Int!\n\tpolicies(query: String, pagination: Pagination): [Policy!]!\n\tpolicyCount(query: String): Int!\n\tpolicyStatus(query: String) : String!\n\tprocessActivityCount: Int!\n\tsecretCount(query: String): Int!\n\tsecrets(query: String, pagination: Pagination): [Secret!]!\n\tserviceAccountID: String!\n\tserviceAccountObject: ServiceAccount\n\tunusedVarSink(query: String): Int\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability!]!\n}\n\ninterface DeploymentEvent {\n    id: ID!\n    name: String!\n    timestamp: Time\n}\n\ntype DeploymentsWithMostSevereViolations {\n\tid: ID!\n\tname: String!\n\tnamespace: String!\n\tclusterName: String!\n\tfailingPolicySeverityCounts: PolicyCounter!\n}\n\ntype DockerfileLineRuleField {\n\tinstruction: String!\n\tvalue: String!\n}\n\ntype DynamicClusterConfig {\n\tadmissionControllerConfig: AdmissionControllerConfig\n\tdisableAuditLogs: Boolean!\n\tregistryOverride: String!\n}\n\ntype Email {\n\tdisableTLS: Boolean!\n\tfrom: String!\n\tpassword: String!\n\tsender: String!\n\tserver: String!\n\tstartTLSAuthMethod: Email_AuthMethod!\n\tusername: String!\n}\n\n\nenum Email_AuthMethod {\n    DISABLED\n    PLAIN\n    LOGIN\n}\n\ntype EmbeddedImageScanComponent {\n\tlicense: License\n\tid: ID!\n\tname: String!\n\tversion: String!\n\ttopVuln: EmbeddedVulnerability\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability]!\n\tvulnCount(query: String, scopeQuery: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tlastScanned: Time\n\timages(query: String, scopeQuery: String, pagination: Pagination): [Image!]!\n\timageCount(query: String, scopeQuery: String): Int!\n\tdeployments(query: String, scopeQuery: String, pagination: Pagination): [Deployment!]!\n\tdeploymentCount(query: String, scopeQuery: String): Int!\n\tactiveState(query: String): ActiveState\n\tnodes(query: String, scopeQuery: String, pagination: Pagination): [Node!]!\n\tnodeCount(query: String, scopeQuery: String): Int!\n\tpriority: Int!\n\tsource: String!\n\tlocation(query: String): String!\n\triskScore: Float!\n\tfixedIn: String!\n\tlayerIndex: Int\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\tunusedVarSink(query: String): Int\n}\n\ntype EmbeddedImageScanComponent_Executable {\n\tdependencies: [String!]!\n\tpath: String!\n}\n\ntype EmbeddedNodeScanComponent {\n\tid: ID!\n\tname: String!\n\tversion: String!\n\ttopVuln: EmbeddedVulnerability\n\tvulns(query: String, pagination: Pagination): [EmbeddedVulnerability]!\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tlastScanned: Time\n\tpriority: Int!\n\triskScore: Float!\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\tunusedVarSink(query: String): Int\n}\n\ntype EmbeddedSecret {\n\tname: String!\n\tpath: String!\n}\n\ntype EmbeddedVulnerability {\n\tid: ID!\n\tcve: String!\n\tcvss: Float!\n\tscoreVersion: String!\n\tvectors: EmbeddedVulnerabilityVectors\n\tlink: String!\n\tsummary: String!\n\tfixedByVersion: String!\n\tisFixable(query: String): Boolean!\n\tlastScanned: Time\n\tcreatedAt: Time\n\tdiscoveredAtImage(query: String): Time\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tcomponentCount(query: String): Int!\n\timages(query: String, pagination: Pagination): [Image!]!\n\timageCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tdeploymentCount(query: String): Int!\n\tnodes(query: String, pagination: Pagination): [Node!]!\n\tnodeCount(query: String): Int!\n\tenvImpact: Float!\n\tseverity: String!\n\tpublishedOn: Time\n\tlastModified: Time\n\timpactScore: Float!\n\tvulnerabilityType: String!\n\tvulnerabilityTypes: [String!]!\n\tsuppressed: Boolean!\n\tsuppressActivation: Time\n\tsuppressExpiry: Time\n\tactiveState(query: String): ActiveState\n\tvulnerabilityState: String!\n\teffectiveVulnerabilityRequest: VulnerabilityRequest\n\tunusedVarSink(query: String): Int\n}\n\nunion EmbeddedVulnerabilityVectors = CVSSV2 | CVSSV3\n\n\nenum EmbeddedVulnerability_ScoreVersion {\n    V2\n    V3\n}\n\n\nenum EmbeddedVulnerability_VulnerabilityType {\n    UNKNOWN_VULNERABILITY\n    IMAGE_VULNERABILITY\n    K8S_VULNERABILITY\n    ISTIO_VULNERABILITY\n    NODE_VULNERABILITY\n    OPENSHIFT_VULNERABILITY\n}\n\n\nenum EnforcementAction {\n    UNSET_ENFORCEMENT\n    SCALE_TO_ZERO_ENFORCEMENT\n    UNSATISFIABLE_NODE_CONSTRAINT_ENFORCEMENT\n    KILL_POD_ENFORCEMENT\n    FAIL_BUILD_ENFORCEMENT\n    FAIL_KUBE_REQUEST_ENFORCEMENT\n    FAIL_DEPLOYMENT_CREATE_ENFORCEMENT\n    FAIL_DEPLOYMENT_UPDATE_ENFORCEMENT\n}\n\n\nenum EventSource {\n    NOT_APPLICABLE\n    DEPLOYMENT_EVENT\n    AUDIT_LOG_EVENT\n}\n\ntype Exclusion {\n\tdeployment: Exclusion_Deployment\n\texpiration: Time\n\timage: Exclusion_Image\n\tname: String!\n}\n\ntype Exclusion_Deployment {\n\tname: String!\n\tscope: Scope\n}\n\ntype Exclusion_Image {\n\tname: String!\n}\n\ntype FalsePositiveRequest {\n}\n\ninput FalsePositiveVulnRequest {\n\tcomment: String\n\tcve: String\n\tscope: VulnReqScope\n}\n\ntype GenerateTokenResponse {\n\tmetadata: TokenMetadata\n\ttoken: String!\n}\n\ntype Generic {\n\tauditLoggingEnabled: Boolean!\n\tcaCert: String!\n\tendpoint: String!\n\textraFields: [KeyValuePair]!\n\theaders: [KeyValuePair]!\n\tpassword: String!\n\tskipTLSVerify: Boolean!\n\tusername: String!\n}\n\ntype GetComplianceRunStatusesResponse {\n\tinvalidRunIds: [String!]!\n\truns: [ComplianceRun]!\n}\n\ntype GetPermissionsResponse {\n\tresourceToAccess: [Label!]!\n}\n\ntype GoogleProviderMetadata {\n\tclusterName: String!\n\tproject: String!\n}\n\ntype Group {\n\tprops: GroupProperties\n\troleName: String!\n}\n\ntype GroupProperties {\n\tauthProviderId: String!\n\tkey: String!\n\tvalue: String!\n}\n\ntype HostMountPolicy {\n}\n\ntype Image {\n\tid: ID!\n\tisClusterLocal: Boolean!\n\tlastUpdated: Time\n\tmetadata: ImageMetadata\n\tname: ImageName\n\tnotPullable: Boolean!\n\tnotes: [Image_Note!]!\n\tpriority: Int!\n\triskScore: Float!\n\tscan: ImageScan\n\tsignature: ImageSignature\n\tsignatureVerificationData: ImageSignatureVerificationData\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\ttopVuln(query: String): EmbeddedVulnerability\n\tunusedVarSink(query: String): Int\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability]!\n\twatchStatus: ImageWatchStatus!\n}\n\ntype ImageComponent {\n\tfixedBy: String!\n\tid: ID!\n\tlicense: License\n\tname: String!\n\toperatingSystem: String!\n\tpriority: Int!\n\triskScore: Float!\n\tsource: SourceType!\n\tversion: String!\n}\n\ntype ImageLayer {\n\tauthor: String!\n\tcreated: Time\n\tempty: Boolean!\n\tinstruction: String!\n\tvalue: String!\n}\n\ntype ImageMetadata {\n\tdataSource: DataSource\n\tlayerShas: [String!]!\n\tv1: V1Metadata\n\tv2: V2Metadata\n}\n\ntype ImageName {\n\tfullName: String!\n\tregistry: String!\n\tremote: String!\n\ttag: String!\n}\n\ntype ImageNamePolicy {\n\tregistry: String!\n\tremote: String!\n\ttag: String!\n}\n\ntype ImagePullSecret {\n\tregistries: [ImagePullSecret_Registry]!\n}\n\ntype ImagePullSecret_Registry {\n\tname: String!\n\tusername: String!\n}\n\ntype ImageScan {\n\tdataSource: DataSource\n\tnotes: [ImageScan_Note!]!\n\toperatingSystem: String!\n\tscanTime: Time\n\tscannerVersion: String!\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n}\n\n\nenum ImageScan_Note {\n    UNSET\n    OS_UNAVAILABLE\n    PARTIAL_SCAN_DATA\n    OS_CVES_UNAVAILABLE\n    OS_CVES_STALE\n    LANGUAGE_CVES_UNAVAILABLE\n    CERTIFIED_RHEL_SCAN_UNAVAILABLE\n}\n\ntype ImageSignature {\n\tfetched: Time\n\tsignatures: [Signature]!\n}\n\ntype ImageSignatureVerificationData {\n\tresults: [ImageSignatureVerificationResult]!\n}\n\ntype ImageSignatureVerificationResult {\n\tdescription: String!\n\tstatus: ImageSignatureVerificationResult_Status!\n\tverificationTime: Time\n\tverifierId: String!\n}\n\n\nenum ImageSignatureVerificationResult_Status {\n    UNSET\n    VERIFIED\n    FAILED_VERIFICATION\n    INVALID_SIGNATURE_ALGO\n    CORRUPTED_SIGNATURE\n    GENERIC_ERROR\n}\n\n\nenum ImageWatchStatus {\n    NOT_WATCHED\n    WATCHED\n}\n\n\nenum Image_Note {\n    MISSING_METADATA\n    MISSING_SCAN_DATA\n    MISSING_SIGNATURE\n    MISSING_SIGNATURE_VERIFICATION_DATA\n}\n\ntype Jira {\n\tdefaultFieldsJson: String!\n\tissueType: String!\n\tpassword: String!\n\tpriorityMappings: [Jira_PriorityMapping]!\n\turl: String!\n\tusername: String!\n}\n\ntype Jira_PriorityMapping {\n\tpriorityName: String!\n\tseverity: Severity!\n}\n\ntype K8SRole {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tclusterRole: Boolean!\n\tcreatedAt: Time\n\tid: ID!\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\trules: [PolicyRule]!\n\tcluster: Cluster!\n\tresources: [String!]!\n\troleNamespace: Namespace\n\tserviceAccountCount(query: String): Int!\n\tserviceAccounts(query: String, pagination: Pagination): [ServiceAccount!]!\n\tsubjectCount(query: String): Int!\n\tsubjects(query: String, pagination: Pagination): [Subject!]!\n\ttype: String!\n\turls: [String!]!\n\tverbs: [String!]!\n}\n\ntype K8SRoleBinding {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tclusterRole: Boolean!\n\tcreatedAt: Time\n\tid: ID!\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\troleId: String!\n\tsubjects: [Subject]!\n}\n\ntype KeyValuePair {\n\tkey: String!\n\tvalue: String!\n}\n\ntype KeyValuePolicy {\n\tenvVarSource: ContainerConfig_EnvironmentConfig_EnvVarSource!\n\tkey: String!\n\tvalue: String!\n}\n\n\nenum L4Protocol {\n    L4_PROTOCOL_UNKNOWN\n    L4_PROTOCOL_TCP\n    L4_PROTOCOL_UDP\n    L4_PROTOCOL_ICMP\n    L4_PROTOCOL_RAW\n    L4_PROTOCOL_SCTP\n    L4_PROTOCOL_ANY\n}\n\ntype Label {\n\tkey: String!\n\tvalue: String!\n}\n\ntype LabelSelector {\n\tmatchLabels: [Label!]!\n\trequirements: [LabelSelector_Requirement]!\n}\n\n\nenum LabelSelector_Operator {\n    UNKNOWN\n    IN\n    NOT_IN\n    EXISTS\n    NOT_EXISTS\n}\n\ntype LabelSelector_Requirement {\n\tkey: String!\n\top: LabelSelector_Operator!\n\tvalues: [String!]!\n}\n\ntype License {\n\tname: String!\n\ttype: String!\n\turl: String!\n}\n\n\nenum LifecycleStage {\n    DEPLOY\n    BUILD\n    RUNTIME\n}\n\n\nenum ListAlert_ResourceType {\n    DEPLOYMENT\n    SECRETS\n    CONFIGMAPS\n}\n\ntype LivenessProbe {\n\tdefined: Boolean!\n}\n\n\nenum ManagerType {\n    MANAGER_TYPE_UNKNOWN\n    MANAGER_TYPE_MANUAL\n    MANAGER_TYPE_HELM_CHART\n    MANAGER_TYPE_KUBERNETES_OPERATOR\n}\n\ntype Metadata {\n\tbuildFlavor: String!\n\tlicenseStatus: Metadata_LicenseStatus!\n\treleaseBuild: Boolean!\n\tversion: String!\n}\n\n\nenum Metadata_LicenseStatus {\n    NONE\n    INVALID\n    EXPIRED\n    RESTARTING\n    VALID\n}\n\ntype MitreAttackVector {\n\ttactic: MitreTactic\n\ttechniques: [MitreTechnique]!\n}\n\ntype MitreTactic {\n\tdescription: String!\n\tid: ID!\n\tname: String!\n}\n\ntype MitreTechnique {\n\tdescription: String!\n\tid: ID!\n\tname: String!\n}\n\ntype Namespace {\n\tmetadata: NamespaceMetadata\n\tnumDeployments: Int!\n\tnumNetworkPolicies: Int!\n\tnumSecrets: Int!\n\tcluster: Cluster!\n\tcomplianceResults(query: String): [ControlResult!]!\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tfailingPolicyCounter(query: String): PolicyCounter\n\timageCount(query: String): Int!\n\timages(query: String, pagination: Pagination): [Image!]!\n\tk8sRoleCount(query: String): Int!\n\tk8sRoles(query: String, pagination: Pagination): [K8SRole!]!\n\tlatestViolation(query: String): Time\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\tpolicies(query: String, pagination: Pagination): [Policy!]!\n\tpolicyCount(query: String): Int!\n\tpolicyStatus(query: String): PolicyStatus!\n\tpolicyStatusOnly(query: String): String!\n\trisk: Risk\n\tsecretCount(query: String): Int!\n\tsecrets(query: String, pagination: Pagination): [Secret!]!\n\tserviceAccountCount(query: String): Int!\n\tserviceAccounts(query: String, pagination: Pagination): [ServiceAccount!]!\n\tsubjectCount(query: String): Int!\n\tsubjects(query: String, pagination: Pagination): [Subject!]!\n\tunusedVarSink(query: String): Int\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability!]!\n}\n\ntype NamespaceMetadata {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tcreationTime: Time\n\tid: ID!\n\tlabels: [Label!]!\n\tname: String!\n\tpriority: Int!\n}\n\ntype NetworkEntityInfo {\n\tdeployment: NetworkEntityInfo_Deployment\n\texternalSource: NetworkEntityInfo_ExternalSource\n\tid: ID!\n\ttype: NetworkEntityInfo_Type!\n\tdesc: NetworkEntityInfoDesc\n}\n\nunion NetworkEntityInfoDesc = NetworkEntityInfo_Deployment | NetworkEntityInfo_ExternalSource\n\ntype NetworkEntityInfo_Deployment {\n\tcluster: String!\n\tlistenPorts: [NetworkEntityInfo_Deployment_ListenPort]!\n\tname: String!\n\tnamespace: String!\n}\n\ntype NetworkEntityInfo_Deployment_ListenPort {\n\tl4Protocol: L4Protocol!\n\tport: Int!\n}\n\ntype NetworkEntityInfo_ExternalSource {\n\tdefault: Boolean!\n\tname: String!\n}\n\n\nenum NetworkEntityInfo_Type {\n    UNKNOWN_TYPE\n    DEPLOYMENT\n    INTERNET\n    LISTEN_ENDPOINT\n    EXTERNAL_SOURCE\n}\n\ntype NetworkFlow {\n\tclusterId: String!\n\tlastSeenTimestamp: Time\n\tprops: NetworkFlowProperties\n}\n\ntype NetworkFlowProperties {\n\tdstEntity: NetworkEntityInfo\n\tdstPort: Int!\n\tl4Protocol: L4Protocol!\n\tsrcEntity: NetworkEntityInfo\n}\n\ntype Node {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tcontainerRuntime: ContainerRuntimeInfo\n\tcontainerRuntimeVersion: String!\n\texternalIpAddresses: [String!]!\n\tid: ID!\n\tinternalIpAddresses: [String!]!\n\tjoinedAt: Time\n\tk8SUpdated: Time\n\tkernelVersion: String!\n\tkubeProxyVersion: String!\n\tkubeletVersion: String!\n\tlabels: [Label!]!\n\tlastUpdated: Time\n\tname: String!\n\toperatingSystem: String!\n\tosImage: String!\n\tpriority: Int!\n\triskScore: Float!\n\tscan: NodeScan\n\ttaints: [Taint]!\n\tcluster: Cluster!\n\tcomplianceResults(query: String): [ControlResult!]!\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedImageScanComponent!]!\n\tcontrolStatus(query: String): String!\n\tcontrols(query: String): [ComplianceControl!]!\n\tfailingControls(query: String): [ComplianceControl!]!\n\tnodeComplianceControlCount(query: String) : ComplianceControlCount!\n\tnodeStatus(query: String): String!\n\tpassingControls(query: String): [ComplianceControl!]!\n\tplottedVulns(query: String): PlottedVulnerabilities!\n\ttopVuln(query: String): EmbeddedVulnerability\n\tunusedVarSink(query: String): Int\n\tvulnCount(query: String): Int!\n\tvulnCounter(query: String): VulnerabilityCounter!\n\tvulns(query: String, scopeQuery: String, pagination: Pagination): [EmbeddedVulnerability]!\n}\n\ntype NodeScan {\n\toperatingSystem: String!\n\tscanTime: Time\n\tcomponentCount(query: String): Int!\n\tcomponents(query: String, pagination: Pagination): [EmbeddedNodeScanComponent!]!\n}\n\ntype Notifier {\n\tawsSecurityHub: AWSSecurityHub\n\tcscc: CSCC\n\temail: Email\n\tgeneric: Generic\n\tid: ID!\n\tjira: Jira\n\tlabelDefault: String!\n\tlabelKey: String!\n\tname: String!\n\tpagerduty: PagerDuty\n\tsplunk: Splunk\n\tsumologic: SumoLogic\n\tsyslog: Syslog\n\ttype: String!\n\tuiEndpoint: String!\n\tconfig: NotifierConfig\n}\n\nunion NotifierConfig = Jira | Email | CSCC | Splunk | PagerDuty | Generic | SumoLogic | AWSSecurityHub | Syslog\n\ntype NumericalPolicy {\n\top: Comparator!\n\tvalue: Float!\n}\n\ntype OrchestratorMetadata {\n\tapiVersions: [String!]!\n\tbuildDate: Time\n\tversion: String!\n\topenshiftVersion: String!\n}\n\ntype PagerDuty {\n\tapiKey: String!\n}\n\ninput Pagination {\n\tlimit: Int\n\toffset: Int\n\tsortOption: SortOption\n}\n\n\nenum PermissionLevel {\n    UNSET\n    NONE\n    DEFAULT\n    ELEVATED_IN_NAMESPACE\n    ELEVATED_CLUSTER_WIDE\n    CLUSTER_ADMIN\n}\n\ntype PermissionPolicy {\n\tpermissionLevel: PermissionLevel!\n}\n\ntype PermissionSet {\n\tdescription: String!\n\tid: ID!\n\tname: String!\n\tresourceToAccess: [Label!]!\n}\n\ntype PlottedVulnerabilities {\n\tbasicVulnCounter: VulnerabilityCounter!\n\tvulns(pagination: Pagination): [EmbeddedVulnerability]!\n}\n\ntype Pod {\n\tclusterId: String!\n\tdeploymentId: String!\n\tid: ID!\n\tliveInstances: [ContainerInstance]!\n\tname: String!\n\tnamespace: String!\n\tstarted: Time\n\tterminatedInstances: [Pod_ContainerInstanceList]!\n\tcontainerCount: Int!\n\tevents: [DeploymentEvent!]!\n}\n\ntype Pod_ContainerInstanceList {\n\tinstances: [ContainerInstance]!\n}\n\ntype Policy {\n\tcategories: [String!]!\n\tcriteriaLocked: Boolean!\n\tdescription: String!\n\tdisabled: Boolean!\n\tenforcementActions: [EnforcementAction!]!\n\teventSource: EventSource!\n\texclusions: [Exclusion]!\n\tfields: PolicyFields\n\tid: ID!\n\tisDefault: Boolean!\n\tlastUpdated: Time\n\tlifecycleStages: [LifecycleStage!]!\n\tmitreAttackVectors: [Policy_MitreAttackVectors]!\n\tmitreVectorsLocked: Boolean!\n\tname: String!\n\tnotifiers: [String!]!\n\tpolicySections: [PolicySection]!\n\tpolicyVersion: String!\n\trationale: String!\n\tremediation: String!\n\tsORTEnforcement: Boolean!\n\tsORTLifecycleStage: String!\n\tsORTName: String!\n\tscope: [Scope]!\n\tseverity: Severity!\n\talertCount(query: String): Int!\n\talerts(query: String, pagination: Pagination): [Alert!]!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\tfailingDeploymentCount(query: String): Int!\n\tfailingDeployments(query: String, pagination: Pagination): [Deployment!]!\n\tfullMitreAttackVectors: [MitreAttackVector!]!\n\tlatestViolation(query: String): Time\n\tpolicyStatus(query: String): String!\n\tunusedVarSink(query: String): Int\n\twhitelists: [Exclusion]!\n}\n\ntype PolicyCounter {\n\ttotal: Int!\n\tlow: Int!\n\tmedium: Int!\n\thigh: Int!\n\tcritical: Int!\n}\n\ntype PolicyFields {\n\taddCapabilities: [String!]!\n\targs: String!\n\tcommand: String!\n\tcomponent: Component\n\tcontainerResourcePolicy: ResourcePolicy\n\tcve: String!\n\tcvss: NumericalPolicy\n\tdirectory: String!\n\tdisallowedAnnotation: KeyValuePolicy\n\tdisallowedImageLabel: KeyValuePolicy\n\tdropCapabilities: [String!]!\n\tenv: KeyValuePolicy\n\tfixedBy: String!\n\thostMountPolicy: HostMountPolicy\n\timageName: ImageNamePolicy\n\timageSignatureVerifiedBy: String!\n\tlineRule: DockerfileLineRuleField\n\tpermissionPolicy: PermissionPolicy\n\tportExposurePolicy: PortExposurePolicy\n\tportPolicy: PortPolicy\n\tprocessPolicy: ProcessPolicy\n\trequiredAnnotation: KeyValuePolicy\n\trequiredImageLabel: KeyValuePolicy\n\trequiredLabel: KeyValuePolicy\n\tuser: String!\n\tvolumePolicy: VolumePolicy\n\timageAgeDays: Int\n\tnoScanExists: Boolean\n\tprivileged: Boolean\n\treadOnlyRootFs: Boolean\n\tscanAgeDays: Int\n\twhitelistEnabled: Boolean!\n}\n\ntype PolicyGroup {\n\tbooleanOperator: BooleanOperator!\n\tfieldName: String!\n\tnegate: Boolean!\n\tvalues: [PolicyValue]!\n}\n\ntype PolicyRule {\n\tapiGroups: [String!]!\n\tnonResourceUrls: [String!]!\n\tresourceNames: [String!]!\n\tresources: [String!]!\n\tverbs: [String!]!\n}\n\ntype PolicySection {\n\tpolicyGroups: [PolicyGroup]!\n\tsectionName: String!\n}\n\ntype PolicyStatus {\n\tstatus: String!\n\tfailingPolicies: [Policy!]!\n}\n\ntype PolicyValue {\n\tvalue: String!\n}\n\ntype PolicyViolationEvent implements DeploymentEvent {\n\tid: ID!\n\tname: String!\n\ttimestamp: Time\n}\n\ntype Policy_MitreAttackVectors {\n\ttactic: String!\n\ttechniques: [String!]!\n}\n\ntype PortConfig {\n\tcontainerPort: Int!\n\texposedPort: Int!\n\texposure: PortConfig_ExposureLevel!\n\texposureInfos: [PortConfig_ExposureInfo]!\n\tname: String!\n\tprotocol: String!\n}\n\ntype PortConfig_ExposureInfo {\n\texternalHostnames: [String!]!\n\texternalIps: [String!]!\n\tlevel: PortConfig_ExposureLevel!\n\tnodePort: Int!\n\tserviceClusterIp: String!\n\tserviceId: String!\n\tserviceName: String!\n\tservicePort: Int!\n}\n\n\nenum PortConfig_ExposureLevel {\n    UNSET\n    EXTERNAL\n    NODE\n    INTERNAL\n    HOST\n    ROUTE\n}\n\ntype PortExposurePolicy {\n\texposureLevels: [PortConfig_ExposureLevel!]!\n}\n\ntype PortPolicy {\n\tport: Int!\n\tprotocol: String!\n}\n\ntype ProcessActivityEvent implements DeploymentEvent {\n\tid: ID!\n\tname: String!\n\ttimestamp: Time\n\targs: String!\n\tuid: Int!\n\tparentName: String\n\tparentUid: Int!\n\twhitelisted: Boolean!\n\tinBaseline: Boolean!\n}\n\ntype ProcessGroup {\n\targs: String!\n\tsignals: [ProcessIndicator]!\n}\n\ntype ProcessIndicator {\n\tclusterId: String!\n\tcontainerName: String!\n\tcontainerStartTime: Time\n\tdeploymentId: String!\n\tid: ID!\n\timageId: String!\n\tnamespace: String!\n\tpodId: String!\n\tpodUid: String!\n\tsignal: ProcessSignal\n}\n\ntype ProcessNameGroup {\n\tgroups: [ProcessGroup]!\n\tname: String!\n\ttimesExecuted: Int!\n}\n\ninput ProcessNoteKey {\n\targs: String!\n\tcontainerName: String!\n\tdeploymentID: String!\n\texecFilePath: String!\n}\n\ntype ProcessPolicy {\n\tancestor: String!\n\targs: String!\n\tname: String!\n\tuid: String!\n}\n\ntype ProcessSignal {\n\targs: String!\n\tcontainerId: String!\n\texecFilePath: String!\n\tgid: Int!\n\tid: ID!\n\tlineage: [String!]!\n\tlineageInfo: [ProcessSignal_LineageInfo]!\n\tname: String!\n\tpid: Int!\n\tscraped: Boolean!\n\ttime: Time\n\tuid: Int!\n}\n\ntype ProcessSignal_LineageInfo {\n\tparentExecFilePath: String!\n\tparentUid: Int!\n}\n\ntype ProviderMetadata {\n\taws: AWSProviderMetadata\n\tazure: AzureProviderMetadata\n\tgoogle: GoogleProviderMetadata\n\tregion: String!\n\tverified: Boolean!\n\tzone: String!\n\tprovider: ProviderMetadataProvider\n}\n\nunion ProviderMetadataProvider = GoogleProviderMetadata | AWSProviderMetadata | AzureProviderMetadata\n\ntype ReadinessProbe {\n\tdefined: Boolean!\n}\n\ntype RequestComment {\n\tcreatedAt: Time\n\tid: ID!\n\tmessage: String!\n\tuser: SlimUser\n}\n\nunion Resource = Deployment | Cluster | Node\n\ntype ResourcePolicy {\n\tcpuResourceLimit: NumericalPolicy\n\tcpuResourceRequest: NumericalPolicy\n\tmemoryResourceLimit: NumericalPolicy\n\tmemoryResourceRequest: NumericalPolicy\n}\n\n\nenum ResourceType {\n    UNSET_RESOURCE_TYPE\n    ALERT\n    PROCESS\n}\n\ntype Resources {\n\tcpuCoresLimit: Float!\n\tcpuCoresRequest: Float!\n\tmemoryMbLimit: Float!\n\tmemoryMbRequest: Float!\n}\n\ntype Risk {\n\tid: ID!\n\tresults: [Risk_Result]!\n\tscore: Float!\n\tsubject: RiskSubject\n}\n\ntype RiskSubject {\n\tclusterId: String!\n\tid: ID!\n\tnamespace: String!\n\ttype: RiskSubjectType!\n}\n\n\nenum RiskSubjectType {\n    UNKNOWN\n    DEPLOYMENT\n    NAMESPACE\n    CLUSTER\n    NODE\n    NODE_COMPONENT\n    IMAGE\n    IMAGE_COMPONENT\n    SERVICEACCOUNT\n}\n\ntype Risk_Result {\n\tfactors: [Risk_Result_Factor]!\n\tname: String!\n\tscore: Float!\n}\n\ntype Risk_Result_Factor {\n\tmessage: String!\n\turl: String!\n}\n\ntype Role {\n\taccessScopeId: String!\n\tdescription: String!\n\tglobalAccess: Access!\n\tname: String!\n\tpermissionSetId: String!\n\tresourceToAccess: [Label!]!\n}\n\ntype Scope {\n\tcluster: String!\n\tlabel: Scope_Label\n\tnamespace: String!\n}\n\ntype Scope_Label {\n\tkey: String!\n\tvalue: String!\n}\n\ntype ScopedPermissions {\n\tscope: String!\n\tpermissions: [StringListEntry!]!\n}\n\n\nenum SearchCategory {\n    SEARCH_UNSET\n    ALERTS\n    IMAGES\n    IMAGE_COMPONENTS\n    IMAGE_VULN_EDGE\n    IMAGE_COMPONENT_EDGE\n    POLICIES\n    DEPLOYMENTS\n    ACTIVE_COMPONENT\n    PODS\n    SECRETS\n    PROCESS_INDICATORS\n    COMPLIANCE\n    CLUSTERS\n    NAMESPACES\n    NODES\n    NODE_VULN_EDGE\n    NODE_COMPONENT_EDGE\n    COMPLIANCE_STANDARD\n    COMPLIANCE_CONTROL_GROUP\n    COMPLIANCE_CONTROL\n    SERVICE_ACCOUNTS\n    ROLES\n    ROLEBINDINGS\n    REPORT_CONFIGURATIONS\n    PROCESS_BASELINES\n    SUBJECTS\n    RISKS\n    VULNERABILITIES\n    CLUSTER_VULNERABILITIES\n    IMAGE_VULNERABILITIES\n    NODE_VULNERABILITIES\n    COMPONENT_VULN_EDGE\n    CLUSTER_VULN_EDGE\n    NETWORK_ENTITY\n    VULN_REQUEST\n}\n\ntype SearchResult {\n\tcategory: SearchCategory!\n\tid: ID!\n\tlocation: String!\n\tname: String!\n\tscore: Float!\n}\n\ntype Secret {\n\tannotations: [Label!]!\n\tclusterId: String!\n\tclusterName: String!\n\tcreatedAt: Time\n\tfiles: [SecretDataFile]!\n\tid: ID!\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\trelationship: SecretRelationship\n\ttype: String!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n}\n\ntype SecretContainerRelationship {\n\tid: ID!\n\tpath: String!\n}\n\ntype SecretDataFile {\n\tcert: Cert\n\timagePullSecret: ImagePullSecret\n\tname: String!\n\ttype: SecretType!\n\tmetadata: SecretDataFileMetadata\n}\n\nunion SecretDataFileMetadata = Cert | ImagePullSecret\n\ntype SecretDeploymentRelationship {\n\tid: ID!\n\tname: String!\n}\n\ntype SecretRelationship {\n\tcontainerRelationships: [SecretContainerRelationship]!\n\tdeploymentRelationships: [SecretDeploymentRelationship]!\n\tid: ID!\n}\n\n\nenum SecretType {\n    UNDETERMINED\n    PUBLIC_CERTIFICATE\n    CERTIFICATE_REQUEST\n    PRIVACY_ENHANCED_MESSAGE\n    OPENSSH_PRIVATE_KEY\n    PGP_PRIVATE_KEY\n    EC_PRIVATE_KEY\n    RSA_PRIVATE_KEY\n    DSA_PRIVATE_KEY\n    CERT_PRIVATE_KEY\n    ENCRYPTED_PRIVATE_KEY\n    IMAGE_PULL_SECRET\n}\n\ntype SecurityContext {\n\taddCapabilities: [String!]!\n\tallowPrivilegeEscalation: Boolean!\n\tdropCapabilities: [String!]!\n\tprivileged: Boolean!\n\treadOnlyRootFilesystem: Boolean!\n\tseccompProfile: SecurityContext_SeccompProfile\n\tselinux: SecurityContext_SELinux\n}\n\ntype SecurityContext_SELinux {\n\tlevel: String!\n\trole: String!\n\ttype: String!\n\tuser: String!\n}\n\ntype SecurityContext_SeccompProfile {\n\tlocalhostProfile: String!\n\ttype: SecurityContext_SeccompProfile_ProfileType!\n}\n\n\nenum SecurityContext_SeccompProfile_ProfileType {\n    UNCONFINED\n    RUNTIME_DEFAULT\n    LOCALHOST\n}\n\ntype SensorDeploymentIdentification {\n\tappNamespace: String!\n\tappNamespaceId: String!\n\tappServiceaccountId: String!\n\tdefaultNamespaceId: String!\n\tk8SNodeName: String!\n\tsystemNamespaceId: String!\n}\n\ntype ServiceAccount {\n\tannotations: [Label!]!\n\tautomountToken: Boolean!\n\tclusterId: String!\n\tclusterName: String!\n\tcreatedAt: Time\n\tid: ID!\n\timagePullSecrets: [String!]!\n\tlabels: [Label!]!\n\tname: String!\n\tnamespace: String!\n\tsecrets: [String!]!\n\tcluster: Cluster!\n\tclusterAdmin: Boolean!\n\tdeploymentCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\timagePullSecretCount: Int!\n\timagePullSecretObjects(query: String): [Secret!]!\n\tk8sRoleCount(query: String): Int!\n\tk8sRoles(query: String, pagination: Pagination): [K8SRole!]!\n\tsaNamespace: Namespace!\n\tscopedPermissions: [ScopedPermissions!]!\n}\n\ntype SetBasedLabelSelector {\n\trequirements: [SetBasedLabelSelector_Requirement]!\n}\n\n\nenum SetBasedLabelSelector_Operator {\n    UNKNOWN\n    IN\n    NOT_IN\n    EXISTS\n    NOT_EXISTS\n}\n\ntype SetBasedLabelSelector_Requirement {\n\tkey: String!\n\top: SetBasedLabelSelector_Operator!\n\tvalues: [String!]!\n}\n\n\nenum Severity {\n    UNSET_SEVERITY\n    LOW_SEVERITY\n    MEDIUM_SEVERITY\n    HIGH_SEVERITY\n    CRITICAL_SEVERITY\n}\n\ntype Signature {\n\tcosign: CosignSignature\n\tsignature: SignatureSignature\n}\n\nunion SignatureSignature = CosignSignature\n\ntype SimpleAccessScope {\n\tdescription: String!\n\tid: ID!\n\tname: String!\n\trules: SimpleAccessScope_Rules\n}\n\ntype SimpleAccessScope_Rules {\n\tclusterLabelSelectors: [SetBasedLabelSelector]!\n\tincludedClusters: [String!]!\n\tincludedNamespaces: [SimpleAccessScope_Rules_Namespace]!\n\tnamespaceLabelSelectors: [SetBasedLabelSelector]!\n}\n\ntype SimpleAccessScope_Rules_Namespace {\n\tclusterName: String!\n\tnamespaceName: String!\n}\n\ntype SlimUser {\n\tid: ID!\n\tname: String!\n}\n\ninput SortOption {\n\tfield: String\n\treversed: Boolean\n}\n\n\nenum SourceType {\n    OS\n    PYTHON\n    JAVA\n    RUBY\n    NODEJS\n    DOTNETCORERUNTIME\n    INFRASTRUCTURE\n}\n\ntype Splunk {\n\tauditLoggingEnabled: Boolean!\n\thttpEndpoint: String!\n\thttpToken: String!\n\tinsecure: Boolean!\n\tsourceTypes: [Label!]!\n\ttruncate: Int!\n}\n\ntype StaticClusterConfig {\n\tadmissionController: Boolean!\n\tadmissionControllerEvents: Boolean!\n\tadmissionControllerUpdates: Boolean!\n\tcentralApiEndpoint: String!\n\tcollectionMethod: CollectionMethod!\n\tcollectorImage: String!\n\tmainImage: String!\n\tslimCollector: Boolean!\n\ttolerationsConfig: TolerationsConfig\n\ttype: ClusterType!\n}\n\ntype StringListEntry {\n\tkey: String!\n\tvalues: [String!]!\n}\n\ntype Subject {\n\tclusterId: String!\n\tclusterName: String!\n\tid: ID!\n\tkind: SubjectKind!\n\tname: String!\n\tnamespace: String!\n\tclusterAdmin: Boolean!\n\tk8sRoleCount(query: String): Int!\n\tk8sRoles(query: String, pagination: Pagination): [K8SRole!]!\n\tscopedPermissions: [ScopedPermissions!]!\n\ttype: String!\n}\n\n\nenum SubjectKind {\n    UNSET_KIND\n    SERVICE_ACCOUNT\n    USER\n    GROUP\n}\n\ntype SumoLogic {\n\thttpSourceAddress: String!\n\tskipTLSVerify: Boolean!\n}\n\ntype Syslog {\n\tlocalFacility: Syslog_LocalFacility!\n\ttcpConfig: Syslog_TCPConfig\n\tendpoint: SyslogEndpoint\n}\n\nunion SyslogEndpoint = Syslog_TCPConfig\n\n\nenum Syslog_LocalFacility {\n    LOCAL0\n    LOCAL1\n    LOCAL2\n    LOCAL3\n    LOCAL4\n    LOCAL5\n    LOCAL6\n    LOCAL7\n}\n\ntype Syslog_TCPConfig {\n\thostname: String!\n\tport: Int!\n\tskipTlsVerify: Boolean!\n\tuseTls: Boolean!\n}\n\ntype Taint {\n\tkey: String!\n\ttaintEffect: TaintEffect!\n\tvalue: String!\n}\n\n\nenum TaintEffect {\n    UNKNOWN_TAINT_EFFECT\n    NO_SCHEDULE_TAINT_EFFECT\n    PREFER_NO_SCHEDULE_TAINT_EFFECT\n    NO_EXECUTE_TAINT_EFFECT\n}\n\ntype TokenMetadata {\n\texpiration: Time\n\tid: ID!\n\tissuedAt: Time\n\tname: String!\n\trevoked: Boolean!\n\trole: String!\n\troles: [String!]!\n}\n\ntype Toleration {\n\tkey: String!\n\toperator: Toleration_Operator!\n\ttaintEffect: TaintEffect!\n\tvalue: String!\n}\n\n\nenum Toleration_Operator {\n    TOLERATION_OPERATION_UNKNOWN\n    TOLERATION_OPERATOR_EXISTS\n    TOLERATION_OPERATOR_EQUAL\n}\n\ntype TolerationsConfig {\n\tdisabled: Boolean!\n}\n\ntype UpgradeProgress {\n\tsince: Time\n\tupgradeState: UpgradeProgress_UpgradeState!\n\tupgradeStatusDetail: String!\n}\n\n\nenum UpgradeProgress_UpgradeState {\n    UPGRADE_INITIALIZING\n    UPGRADER_LAUNCHING\n    UPGRADER_LAUNCHED\n    PRE_FLIGHT_CHECKS_COMPLETE\n    UPGRADE_OPERATIONS_DONE\n    UPGRADE_COMPLETE\n    UPGRADE_INITIALIZATION_ERROR\n    PRE_FLIGHT_CHECKS_FAILED\n    UPGRADE_ERROR_ROLLING_BACK\n    UPGRADE_ERROR_ROLLED_BACK\n    UPGRADE_ERROR_ROLLBACK_FAILED\n    UPGRADE_ERROR_UNKNOWN\n    UPGRADE_TIMED_OUT\n}\n\ntype V1Metadata {\n\tauthor: String!\n\tcommand: [String!]!\n\tcreated: Time\n\tdigest: String!\n\tentrypoint: [String!]!\n\tlabels: [Label!]!\n\tlayers: [ImageLayer]!\n\tuser: String!\n\tvolumes: [String!]!\n}\n\ntype V2Metadata {\n\tdigest: String!\n}\n\n\nenum ViolationState {\n    ACTIVE\n    SNOOZED\n    RESOLVED\n    ATTEMPTED\n}\n\ntype Volume {\n\tdestination: String!\n\tmountPropagation: Volume_MountPropagation!\n\tname: String!\n\treadOnly: Boolean!\n\tsource: String!\n\ttype: String!\n}\n\ntype VolumePolicy {\n\tdestination: String!\n\tname: String!\n\tsource: String!\n\ttype: String!\n}\n\n\nenum Volume_MountPropagation {\n    NONE\n    HOST_TO_CONTAINER\n    BIDIRECTIONAL\n}\n\ninput VulnReqExpiry {\n\texpiresOn: Time\n\texpiresWhenFixed: Boolean\n}\n\ninput VulnReqGlobalScope {\n\timages: VulnReqImageScope\n}\n\ninput VulnReqImageScope {\n\tregistry: String\n\tremote: String\n\ttag: String\n}\n\ninput VulnReqScope {\n\tglobalScope: VulnReqGlobalScope\n\timageScope: VulnReqImageScope\n}\n\ntype VulnerabilityCounter {\n\tall: VulnerabilityFixableCounterResolver!\n\tlow: VulnerabilityFixableCounterResolver!\n\tmoderate: VulnerabilityFixableCounterResolver!\n\timportant: VulnerabilityFixableCounterResolver!\n\tcritical: VulnerabilityFixableCounterResolver!\n}\n\ntype VulnerabilityFixableCounterResolver {\n\ttotal: Int!\n\tfixable: Int!\n}\n\ntype VulnerabilityRequest {\n\tid: ID!\n\ttargetState: String!\n\tstatus: String!\n\texpired: Boolean!\n\trequestor: SlimUser\n\tapprovers: [SlimUser!]!\n\tcreatedAt: Time\n\tLastUpdated: Time\n\tcomments: [RequestComment!]!\n\tscope: VulnerabilityRequest_Scope\n\tdeferralReq: DeferralRequest\n\tfalsePositiveReq: FalsePositiveRequest\n\tupdatedDeferralReq: DeferralRequest\n\tcves: VulnerabilityRequest_CVEs\n\tdeploymentCount(query: String): Int!\n\timageCount(query: String): Int!\n\tdeployments(query: String, pagination: Pagination): [Deployment!]!\n\timages(query: String, pagination: Pagination): [Image!]!\n}\n\ntype VulnerabilityRequest_CVEs {\n\tids: [String!]!\n}\n\ntype VulnerabilityRequest_Scope {\n\tglobalScope: VulnerabilityRequest_Scope_Global\n\timageScope: VulnerabilityRequest_Scope_Image\n\tinfo: VulnerabilityRequest_ScopeInfo\n}\n\nunion VulnerabilityRequest_ScopeInfo = VulnerabilityRequest_Scope_Image | VulnerabilityRequest_Scope_Global\n\ntype VulnerabilityRequest_Scope_Global {\n}\n\ntype VulnerabilityRequest_Scope_Image {\n\tregistry: String!\n\tremote: String!\n\ttag: String!\n}\n\n\nenum VulnerabilitySeverity {\n    UNKNOWN_VULNERABILITY_SEVERITY\n    LOW_VULNERABILITY_SEVERITY\n    MODERATE_VULNERABILITY_SEVERITY\n    IMPORTANT_VULNERABILITY_SEVERITY\n    CRITICAL_VULNERABILITY_SEVERITY\n}\n\n\nenum VulnerabilityState {\n    OBSERVED\n    DEFERRED\n    FALSE_POSITIVE\n}\n\nscalar Time\n"
func truncateResults(results []*storage.ComplianceAggregation_Result, domainMap map[*storage.ComplianceAggregation_Result]*storage.ComplianceDomain, collapseBy storage.ComplianceAggregation_Scope) ([]*storage.ComplianceAggregation_Result, map[*storage.ComplianceAggregation_Result]*storage.ComplianceDomain, string) {
	if len(results) == 0 {
		return results, domainMap, ""
	}
	// If the collapseBy is not contained in the result keys do not truncate
	validCollapseBy, collapseIndex := validateCollapseBy(results[0].GetAggregationKeys(), collapseBy)
	if !validCollapseBy {
		return results, domainMap, ""
	}

	collapsedResults := make(map[string][]*storage.ComplianceAggregation_Result)
	for _, result := range results {
		collapsedResults[result.AggregationKeys[collapseIndex].Id] = append(collapsedResults[result.AggregationKeys[collapseIndex].Id], result)
	}

	if len(collapsedResults) <= aggregationLimit {
		return results, domainMap, ""
	}

	var truncatedResults []*storage.ComplianceAggregation_Result
	numResults := 0
	for _, collapsedList := range collapsedResults {
		truncatedResults = append(truncatedResults, collapsedList...)
		numResults++
		if numResults == aggregationLimit {
			break
		}
	}

	truncatedDomainMap := make(map[*storage.ComplianceAggregation_Result]*storage.ComplianceDomain, len(truncatedResults))
	for _, result := range truncatedResults {
		truncatedDomainMap[result] = domainMap[result]
	}

	errMsg := fmt.Sprintf("The following results only contain the first %d of %d %ss. Use search queries to reduce the result set size.", aggregationLimit, len(collapsedResults), strings.ToLower(collapseBy.String()))

	return truncatedResults, truncatedDomainMap, errMsg
}

func validateCollapseBy(scopes []*storage.ComplianceAggregation_AggregationKey, collapseBy storage.ComplianceAggregation_Scope) (bool, int) {
	if collapseBy == storage.ComplianceAggregation_UNKNOWN {
		return false, -1
	}
	for i, scope := range scopes {
		if collapseBy == scope.Scope {
			return true, i
		}
	}
	return false, -1
}

type complianceDomainKeyResolver struct {
	wrapped interface{}
}

func newComplianceDomainKeyResolverWrapped(ctx context.Context, root *Resolver, domain *storage.ComplianceDomain, key *storage.ComplianceAggregation_AggregationKey) interface{} {
	switch key.GetScope() {
	case storage.ComplianceAggregation_CLUSTER:
		if domain.GetCluster() != nil {
			return &complianceDomain_ClusterResolver{ctx, root, domain.GetCluster()}
		}
	case storage.ComplianceAggregation_DEPLOYMENT:
		deployment, found := domain.GetDeployments()[key.GetId()]
		if found {
			return &complianceDomain_DeploymentResolver{ctx, root, deployment}
		}
	case storage.ComplianceAggregation_NAMESPACE:
		receivedNS, found, err := namespace.ResolveByID(ctx, key.GetId(), root.NamespaceDataStore,
			root.DeploymentDataStore, root.SecretsDataStore, root.NetworkPoliciesStore)
		if err == nil && found {
			return &namespaceResolver{ctx, root, receivedNS}
		}
	case storage.ComplianceAggregation_NODE:
		node, found := domain.GetNodes()[key.GetId()]
		if found {
			return &complianceDomain_NodeResolver{ctx, root, node}
		}
	case storage.ComplianceAggregation_STANDARD:
		standard, found, err := root.ComplianceStandardStore.StandardMetadata(key.GetId())
		if err == nil && found {
			return &complianceStandardMetadataResolver{ctx, root, standard}
		}
	case storage.ComplianceAggregation_CONTROL:
		controlID := key.GetId()
		control := root.ComplianceStandardStore.Control(controlID)
		if control != nil {
			return &complianceControlResolver{
				root: root,
				data: control,
			}
		}
	case storage.ComplianceAggregation_CATEGORY:
		groupID := key.GetId()
		control := root.ComplianceStandardStore.Group(groupID)
		if control != nil {
			return &complianceControlGroupResolver{
				root: root,
				data: control,
			}
		}
	}
	return nil
}

func (resolver *complianceDomainKeyResolver) ToComplianceDomain_Cluster() (cluster *complianceDomain_ClusterResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceDomain_ClusterResolver)
	return r, ok
}

func (resolver *complianceDomainKeyResolver) ToComplianceDomain_Deployment() (deployment *complianceDomain_DeploymentResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceDomain_DeploymentResolver)
	return r, ok
}

func (resolver *complianceDomainKeyResolver) ToNamespace() (*namespaceResolver, bool) {
	r, ok := resolver.wrapped.(*namespaceResolver)
	return r, ok
}

func (resolver *complianceDomainKeyResolver) ToComplianceDomain_Node() (node *complianceDomain_NodeResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceDomain_NodeResolver)
	return r, ok
}

func (resolver *complianceDomainKeyResolver) ToComplianceStandardMetadata() (standard *complianceStandardMetadataResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceStandardMetadataResolver)
	return r, ok
}

// ToComplianceControl returns a resolver for a control if the domain key refers to a control and it exists
func (resolver *complianceDomainKeyResolver) ToComplianceControl() (control *complianceControlResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceControlResolver)
	return r, ok
}

// ToComplianceControlGroup returns a resolver for a group if the domain key refers to a control group and it exists
func (resolver *complianceDomainKeyResolver) ToComplianceControlGroup() (group *complianceControlGroupResolver, found bool) {
	r, ok := resolver.wrapped.(*complianceControlGroupResolver)
	return r, ok
}

// ComplianceDomain returns a graphql resolver that loads the underlying object for an aggregation key
func (resolver *complianceAggregationResultWithDomainResolver) Keys(ctx context.Context) ([]*complianceDomainKeyResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Compliance, "Keys")

	output := make([]*complianceDomainKeyResolver, len(resolver.data.AggregationKeys))
	for i, v := range resolver.data.AggregationKeys {
		wrapped := newComplianceDomainKeyResolverWrapped(ctx, resolver.root, resolver.domain, v)
		output[i] = &complianceDomainKeyResolver{
			wrapped: wrapped,
		}
	}
	return output, nil
}

// ComplianceResults returns graphql resolvers for all matching compliance results
func (resolver *Resolver) ComplianceResults(ctx context.Context, query RawQuery) ([]*complianceControlResultResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Compliance, "ComplianceResults")

	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	q, err := query.AsV1QueryOrEmpty()
	if err != nil {
		return nil, err
	}
	return resolver.wrapComplianceControlResults(
		resolver.ComplianceDataStore.QueryControlResults(ctx, q))
}

func (resolver *complianceStandardMetadataResolver) Controls(ctx context.Context) ([]*complianceControlResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	return resolver.root.wrapComplianceControls(
		resolver.root.ComplianceStandardStore.Controls(resolver.data.GetId()))
}

func (resolver *complianceStandardMetadataResolver) Groups(ctx context.Context) ([]*complianceControlGroupResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	return resolver.root.wrapComplianceControlGroups(
		resolver.root.ComplianceStandardStore.Groups(resolver.data.GetId()))
}

type controlResultResolver struct {
	root       *Resolver
	controlID  string
	value      *storage.ComplianceResultValue
	deployment *storage.ComplianceDomain_Deployment
	cluster    *storage.ComplianceDomain_Cluster
	node       *storage.ComplianceDomain_Node
}

type bulkControlResults []*controlResultResolver

func newBulkControlResults() *bulkControlResults {
	output := make(bulkControlResults, 0)
	return &output
}

func (container *bulkControlResults) addDeploymentData(root *Resolver, results []*storage.ComplianceRunResults, filter func(*storage.ComplianceDomain_Deployment, *v1.ComplianceControl) bool) {
	for _, runResult := range results {
		for did, res := range runResult.GetDeploymentResults() {
			deployment := runResult.GetDomain().GetDeployments()[did]
			results := res.GetControlResults()
			for controlID, result := range results {
				if filter == nil || filter(deployment, root.ComplianceStandardStore.Control(controlID)) {
					*container = append(*container, &controlResultResolver{
						root:       root,
						controlID:  controlID,
						value:      result,
						deployment: deployment,
					})
				}
			}
		}
	}
}

func (container *bulkControlResults) addClusterData(root *Resolver, results []*storage.ComplianceRunResults, filter func(control *v1.ComplianceControl) bool) {
	for _, runResult := range results {
		res := runResult.GetClusterResults()
		results := res.GetControlResults()
		for controlID, result := range results {
			if filter == nil || filter(root.ComplianceStandardStore.Control(controlID)) {
				*container = append(*container, &controlResultResolver{
					root:      root,
					controlID: controlID,
					value:     result,
					cluster:   runResult.GetDomain().GetCluster(),
				})
			}
		}
	}
}

func (container *bulkControlResults) addNodeData(root *Resolver, results []*storage.ComplianceRunResults, filter func(node *storage.ComplianceDomain_Node, control *v1.ComplianceControl) bool) {
	for _, runResult := range results {
		for nodeID, res := range runResult.GetNodeResults() {
			node := runResult.GetDomain().GetNodes()[nodeID]
			results := res.GetControlResults()
			for controlID, result := range results {
				if filter == nil || filter(node, root.ComplianceStandardStore.Control(controlID)) {
					*container = append(*container, &controlResultResolver{
						root:      root,
						controlID: controlID,
						value:     result,
						node:      node,
					})
				}
			}
		}
	}
}

func (resolver *controlResultResolver) Resource(ctx context.Context) *controlResultResolver {
	return resolver
}

func (resolver *controlResultResolver) Control(ctx context.Context) (*complianceControlResolver, error) {
	return resolver.root.ComplianceControl(ctx, struct{ graphql.ID }{graphql.ID(resolver.controlID)})
}

func (resolver *controlResultResolver) ToComplianceDomain_Deployment() (*complianceDomain_DeploymentResolver, bool) {
	if resolver.deployment == nil {
		return nil, false
	}
	return &complianceDomain_DeploymentResolver{nil, resolver.root, resolver.deployment}, true
}

func (resolver *controlResultResolver) ToComplianceDomain_Cluster() (*complianceDomain_ClusterResolver, bool) {
	if resolver.cluster == nil {
		return nil, false
	}
	return &complianceDomain_ClusterResolver{root: resolver.root, data: resolver.cluster}, true
}

func (resolver *controlResultResolver) ToComplianceDomain_Node() (*complianceDomain_NodeResolver, bool) {
	if resolver.node == nil {
		return nil, false
	}
	return &complianceDomain_NodeResolver{root: resolver.root, data: resolver.node}, true
}

func (resolver *controlResultResolver) Value(ctx context.Context) *complianceResultValueResolver {
	return &complianceResultValueResolver{
		root: resolver.root,
		data: resolver.value,
	}
}

func (resolver *complianceStandardMetadataResolver) ComplianceResults(ctx context.Context, args RawQuery) ([]*controlResultResolver, error) {
	defer metrics.SetGraphQLOperationDurationTime(time.Now(), pkgMetrics.Compliance, "ComplianceResults")

	if err := readCompliance(ctx); err != nil {
		return nil, err
	}

	runResults, err := resolver.root.ComplianceAggregator.GetResultsWithEvidence(ctx, args.String())
	if err != nil {
		return nil, err
	}
	output := newBulkControlResults()
	output.addClusterData(resolver.root, runResults, nil)
	output.addDeploymentData(resolver.root, runResults, nil)
	output.addNodeData(resolver.root, runResults, nil)

	return *output, nil
}

func (resolver *complianceControlResolver) ComplianceResults(ctx context.Context, args RawQuery) ([]*controlResultResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	runResults, err := resolver.root.ComplianceAggregator.GetResultsWithEvidence(ctx, args.String())
	if err != nil {
		return nil, err
	}
	output := newBulkControlResults()
	output.addClusterData(resolver.root, runResults, func(control *v1.ComplianceControl) bool {
		return control.GetId() == resolver.data.GetId()
	})
	output.addDeploymentData(resolver.root, runResults, func(deployment *storage.ComplianceDomain_Deployment, control *v1.ComplianceControl) bool {
		return control.GetId() == resolver.data.GetId()
	})
	output.addNodeData(resolver.root, runResults, func(node *storage.ComplianceDomain_Node, control *v1.ComplianceControl) bool {
		return control.GetId() == resolver.data.GetId()
	})

	return *output, nil
}

func (resolver *complianceControlResolver) ComplianceControlEntities(ctx context.Context, args struct{ ClusterID graphql.ID }) ([]*nodeResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	clusterID := string(args.ClusterID)
	standardIDs, err := getStandardIDs(ctx, resolver.root.ComplianceStandardStore)
	if err != nil {
		return nil, err
	}
	hasComplianceSuccessfullyRun, err := resolver.root.ComplianceDataStore.IsComplianceRunSuccessfulOnCluster(ctx, clusterID, standardIDs)
	if err != nil || !hasComplianceSuccessfullyRun {
		return nil, err
	}
	store, err := resolver.root.NodeGlobalDataStore.GetClusterNodeStore(ctx, clusterID, false)
	if err != nil {
		return nil, err
	}
	return resolver.root.wrapNodes(store.ListNodes())
}

func (resolver *complianceControlResolver) ComplianceControlNodeCount(ctx context.Context, args RawQuery) (*complianceControlNodeCountResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	nr := complianceControlNodeCountResolver{failingCount: 0, passingCount: 0, unknownCount: 0}
	standardIDs, err := getStandardIDs(ctx, resolver.root.ComplianceStandardStore)
	if err != nil {
		return nil, err
	}
	clusterIDs, err := resolver.getClusterIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, clusterID := range clusterIDs {
		rs, ok, err := resolver.getNodeControlAggregationResults(ctx, clusterID, standardIDs, args)
		if !ok || err != nil {
			return nil, err
		}
		ret := getComplianceControlNodeCountFromAggregationResults(rs)
		nr.failingCount += ret.FailingCount()
		nr.passingCount += ret.PassingCount()
		nr.unknownCount += ret.UnknownCount()
	}
	return &nr, nil
}

func (resolver *complianceControlResolver) ComplianceControlNodes(ctx context.Context, args RawQuery) ([]*nodeResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	var ret []*nodeResolver
	standardIDs, err := getStandardIDs(ctx, resolver.root.ComplianceStandardStore)
	if err != nil {
		return nil, err
	}
	clusterIDs, err := resolver.getClusterIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, clusterID := range clusterIDs {
		rs, ok, err := resolver.getNodeControlAggregationResults(ctx, clusterID, standardIDs, args)
		if !ok || err != nil {
			return nil, err
		}
		ds, err := resolver.root.NodeGlobalDataStore.GetClusterNodeStore(ctx, clusterID, false)
		if err != nil {
			return nil, err
		}
		resolvers, err := resolver.root.wrapNodes(getResultNodesFromAggregationResults(rs, any, ds))
		if err != nil {
			return nil, err
		}
		ret = append(ret, resolvers...)
	}
	return ret, nil
}

func (resolver *complianceControlResolver) ComplianceControlFailingNodes(ctx context.Context, args RawQuery) ([]*nodeResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	var ret []*nodeResolver
	standardIDs, err := getStandardIDs(ctx, resolver.root.ComplianceStandardStore)
	if err != nil {
		return nil, err
	}
	clusterIDs, err := resolver.getClusterIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, clusterID := range clusterIDs {
		rs, ok, err := resolver.getNodeControlAggregationResults(ctx, clusterID, standardIDs, args)
		if !ok || err != nil {
			return nil, err
		}
		ds, err := resolver.root.NodeGlobalDataStore.GetClusterNodeStore(ctx, clusterID, false)
		if err != nil {
			return nil, err
		}
		resolvers, err := resolver.root.wrapNodes(getResultNodesFromAggregationResults(rs, failing, ds))
		if err != nil {
			return nil, err
		}
		ret = append(ret, resolvers...)
	}
	return ret, nil
}

func (resolver *complianceControlResolver) ComplianceControlPassingNodes(ctx context.Context, args RawQuery) ([]*nodeResolver, error) {
	if err := readCompliance(ctx); err != nil {
		return nil, err
	}
	standardIDs, err := getStandardIDs(ctx, resolver.root.ComplianceStandardStore)
	if err != nil {
		return nil, err
	}
	var ret []*nodeResolver
	clusterIDs, err := resolver.getClusterIDs(ctx)
	if err != nil {
		return nil, err
	}
	for _, clusterID := range clusterIDs {
		rs, ok, err := resolver.getNodeControlAggregationResults(ctx, clusterID, standardIDs, args)
		if !ok || err != nil {
			return nil, err
		}
		ds, err := resolver.root.NodeGlobalDataStore.GetClusterNodeStore(ctx, clusterID, false)
		if err != nil {
			return nil, err
		}
		resolvers, err := resolver.root.wrapNodes(getResultNodesFromAggregationResults(rs, passing, ds))
		if err != nil {
			return nil, err
		}
		ret = append(ret, resolvers...)
	}
	return ret, nil
}

func (resolver *complianceControlResolver) getNodeControlAggregationResults(ctx context.Context, clusterID string, standardIDs []string, args RawQuery) ([]*storage.ComplianceAggregation_Result, bool, error) {
	hasComplianceSuccessfullyRun, err := resolver.root.ComplianceDataStore.IsComplianceRunSuccessfulOnCluster(ctx, clusterID, standardIDs)
	if err != nil || !hasComplianceSuccessfullyRun {
		return nil, false, err
	}
	query, err := search.NewQueryBuilder().AddExactMatches(search.ClusterID, clusterID).
		AddExactMatches(search.ControlID, resolver.data.GetId()).RawQuery()
	if err != nil {
		return nil, false, err
	}
	if args.Query != nil {
		query = strings.Join([]string{query, *(args.Query)}, "+")
	}
	rs, _, _, err := resolver.root.ComplianceAggregator.Aggregate(ctx, query, []storage.ComplianceAggregation_Scope{storage.ComplianceAggregation_CONTROL, storage.ComplianceAggregation_NODE}, storage.ComplianceAggregation_NODE)
	if err != nil {
		return nil, false, err
	}
	return rs, true, nil
}

func getResultNodesFromAggregationResults(results []*storage.ComplianceAggregation_Result, nodeType resultType, ds datastore.DataStore) ([]*storage.Node, error) {
	if ds == nil {
		return nil, errors.Wrap(errors.New("empty node datastore encountered"), "argument ds is nil")
	}
	var nodes []*storage.Node
	for _, r := range results {
		if (nodeType == passing && r.GetNumPassing() == 0) || (nodeType == failing && r.GetNumFailing() == 0) {
			continue
		}
		nodeID, err := getScopeIDFromAggregationResult(r, storage.ComplianceAggregation_NODE)
		if err != nil {
			continue
		}
		node, err := ds.GetNode(nodeID)
		if err != nil {
			continue
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

type resultType int

const (
	failing resultType = iota
	passing
	any
)

func getScopeIDFromAggregationResult(result *storage.ComplianceAggregation_Result, scope storage.ComplianceAggregation_Scope) (string, error) {
	if result == nil {
		return "", errors.New("empty aggregation result encountered: compliance aggregation result is nil")
	}
	for _, k := range result.GetAggregationKeys() {
		if k.Scope == scope {
			return k.GetId(), nil
		}
	}
	return "", errors.New("bad arguments: scope was not one of the aggregation keys")
}

type complianceControlNodeCountResolver struct {
	failingCount int32
	passingCount int32
	unknownCount int32
}

func (resolver *complianceControlNodeCountResolver) FailingCount() int32 {
	return resolver.failingCount
}

func (resolver *complianceControlNodeCountResolver) PassingCount() int32 {
	return resolver.passingCount
}

func (resolver *complianceControlNodeCountResolver) UnknownCount() int32 {
	return resolver.unknownCount
}

// ComplianceControlWithControlStatusResolver represents a control with its status across clusters
type ComplianceControlWithControlStatusResolver struct {
	complianceControl *complianceControlResolver
	controlStatus     string
}

// ComplianceControl returns a control of ComplianceControlWithControlStatusResolver
func (c *ComplianceControlWithControlStatusResolver) ComplianceControl() *complianceControlResolver {
	if c == nil {
		return nil
	}
	return c.complianceControl
}

// ControlStatus returns a control status of ComplianceControlWithControlStatusResolver
func (c *ComplianceControlWithControlStatusResolver) ControlStatus() string {
	if c == nil {
		return ""
	}
	return c.controlStatus
}
