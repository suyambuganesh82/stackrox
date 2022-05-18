// Code generated by pg-bindings generator. DO NOT EDIT.

package postgres

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/pkg/errors"
	"github.com/stackrox/rox/central/metrics"
	pkgSchema "github.com/stackrox/rox/central/postgres/schema"
	"github.com/stackrox/rox/central/role/resources"
	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/generated/storage"
	"github.com/stackrox/rox/pkg/auth/permissions"
	"github.com/stackrox/rox/pkg/logging"
	ops "github.com/stackrox/rox/pkg/metrics"
	"github.com/stackrox/rox/pkg/postgres/pgutils"
	"github.com/stackrox/rox/pkg/sac"
	"github.com/stackrox/rox/pkg/search"
	"github.com/stackrox/rox/pkg/search/postgres"
)

const (
	baseTable = "deployments"

	getStmt     = "SELECT serialized FROM deployments WHERE Id = $1"
	deleteStmt  = "DELETE FROM deployments WHERE Id = $1"
	walkStmt    = "SELECT serialized FROM deployments"
	getManyStmt = "SELECT serialized FROM deployments WHERE Id = ANY($1::text[])"

	deleteManyStmt = "DELETE FROM deployments WHERE Id = ANY($1::text[])"

	batchAfter = 100

	// using copyFrom, we may not even want to batch.  It would probably be simpler
	// to deal with failures if we just sent it all.  Something to think about as we
	// proceed and move into more e2e and larger performance testing
	batchSize = 10000
)

var (
	log            = logging.LoggerForModule()
	schema         = pkgSchema.DeploymentsSchema
	targetResource = resources.Deployment
)

type Store interface {
	Count(ctx context.Context) (int, error)
	Exists(ctx context.Context, id string) (bool, error)
	Get(ctx context.Context, id string) (*storage.Deployment, bool, error)
	Upsert(ctx context.Context, obj *storage.Deployment) error
	UpsertMany(ctx context.Context, objs []*storage.Deployment) error
	Delete(ctx context.Context, id string) error
	GetIDs(ctx context.Context) ([]string, error)
	GetMany(ctx context.Context, ids []string) ([]*storage.Deployment, []int, error)
	DeleteMany(ctx context.Context, ids []string) error

	Walk(ctx context.Context, fn func(obj *storage.Deployment) error) error

	AckKeysIndexed(ctx context.Context, keys ...string) error
	GetKeysToIndex(ctx context.Context) ([]string, error)
}

type storeImpl struct {
	db *pgxpool.Pool

	// Lock since copyFrom requires a delete first before being executed we can get in odd states if
	// multiple processes are trying to work on the same subsets of rows.
	mutex sync.Mutex
}

// New returns a new Store instance using the provided sql instance.
func New(ctx context.Context, db *pgxpool.Pool) Store {
	pgutils.CreateTable(ctx, db, pkgSchema.CreateTableNamespacesStmt)
	pgutils.CreateTable(ctx, db, pkgSchema.CreateTableDeploymentsStmt)

	return &storeImpl{
		db: db,
	}
}

func insertIntoDeployments(ctx context.Context, tx pgx.Tx, obj *storage.Deployment) error {

	serialized, marshalErr := obj.Marshal()
	if marshalErr != nil {
		return marshalErr
	}

	values := []interface{}{
		// parent primary keys start
		obj.GetId(),
		obj.GetName(),
		obj.GetType(),
		obj.GetNamespace(),
		obj.GetNamespaceId(),
		obj.GetOrchestratorComponent(),
		obj.GetLabels(),
		obj.GetPodLabels(),
		pgutils.NilOrTime(obj.GetCreated()),
		obj.GetClusterId(),
		obj.GetClusterName(),
		obj.GetAnnotations(),
		obj.GetPriority(),
		obj.GetImagePullSecrets(),
		obj.GetServiceAccount(),
		obj.GetServiceAccountPermissionLevel(),
		obj.GetRiskScore(),
		obj.GetProcessTags(),
		serialized,
	}

	finalStr := "INSERT INTO deployments (Id, Name, Type, Namespace, NamespaceId, OrchestratorComponent, Labels, PodLabels, Created, ClusterId, ClusterName, Annotations, Priority, ImagePullSecrets, ServiceAccount, ServiceAccountPermissionLevel, RiskScore, ProcessTags, serialized) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19) ON CONFLICT(Id) DO UPDATE SET Id = EXCLUDED.Id, Name = EXCLUDED.Name, Type = EXCLUDED.Type, Namespace = EXCLUDED.Namespace, NamespaceId = EXCLUDED.NamespaceId, OrchestratorComponent = EXCLUDED.OrchestratorComponent, Labels = EXCLUDED.Labels, PodLabels = EXCLUDED.PodLabels, Created = EXCLUDED.Created, ClusterId = EXCLUDED.ClusterId, ClusterName = EXCLUDED.ClusterName, Annotations = EXCLUDED.Annotations, Priority = EXCLUDED.Priority, ImagePullSecrets = EXCLUDED.ImagePullSecrets, ServiceAccount = EXCLUDED.ServiceAccount, ServiceAccountPermissionLevel = EXCLUDED.ServiceAccountPermissionLevel, RiskScore = EXCLUDED.RiskScore, ProcessTags = EXCLUDED.ProcessTags, serialized = EXCLUDED.serialized"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	var query string

	for childIdx, child := range obj.GetContainers() {
		if err := insertIntoDeploymentsContainers(ctx, tx, child, obj.GetId(), childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Containers where deployments_Id = $1 AND idx >= $2"
	_, err = tx.Exec(ctx, query, obj.GetId(), len(obj.GetContainers()))
	if err != nil {
		return err
	}
	for childIdx, child := range obj.GetPorts() {
		if err := insertIntoDeploymentsPorts(ctx, tx, child, obj.GetId(), childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Ports where deployments_Id = $1 AND idx >= $2"
	_, err = tx.Exec(ctx, query, obj.GetId(), len(obj.GetPorts()))
	if err != nil {
		return err
	}
	return nil
}

func insertIntoDeploymentsContainers(ctx context.Context, tx pgx.Tx, obj *storage.Container, deployments_Id string, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		idx,
		obj.GetImage().GetId(),
		obj.GetImage().GetName().GetRegistry(),
		obj.GetImage().GetName().GetRemote(),
		obj.GetImage().GetName().GetTag(),
		obj.GetImage().GetName().GetFullName(),
		obj.GetSecurityContext().GetPrivileged(),
		obj.GetSecurityContext().GetDropCapabilities(),
		obj.GetSecurityContext().GetAddCapabilities(),
		obj.GetSecurityContext().GetReadOnlyRootFilesystem(),
		obj.GetResources().GetCpuCoresRequest(),
		obj.GetResources().GetCpuCoresLimit(),
		obj.GetResources().GetMemoryMbRequest(),
		obj.GetResources().GetMemoryMbLimit(),
	}

	finalStr := "INSERT INTO deployments_Containers (deployments_Id, idx, Image_Id, Image_Name_Registry, Image_Name_Remote, Image_Name_Tag, Image_Name_FullName, SecurityContext_Privileged, SecurityContext_DropCapabilities, SecurityContext_AddCapabilities, SecurityContext_ReadOnlyRootFilesystem, Resources_CpuCoresRequest, Resources_CpuCoresLimit, Resources_MemoryMbRequest, Resources_MemoryMbLimit) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) ON CONFLICT(deployments_Id, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, idx = EXCLUDED.idx, Image_Id = EXCLUDED.Image_Id, Image_Name_Registry = EXCLUDED.Image_Name_Registry, Image_Name_Remote = EXCLUDED.Image_Name_Remote, Image_Name_Tag = EXCLUDED.Image_Name_Tag, Image_Name_FullName = EXCLUDED.Image_Name_FullName, SecurityContext_Privileged = EXCLUDED.SecurityContext_Privileged, SecurityContext_DropCapabilities = EXCLUDED.SecurityContext_DropCapabilities, SecurityContext_AddCapabilities = EXCLUDED.SecurityContext_AddCapabilities, SecurityContext_ReadOnlyRootFilesystem = EXCLUDED.SecurityContext_ReadOnlyRootFilesystem, Resources_CpuCoresRequest = EXCLUDED.Resources_CpuCoresRequest, Resources_CpuCoresLimit = EXCLUDED.Resources_CpuCoresLimit, Resources_MemoryMbRequest = EXCLUDED.Resources_MemoryMbRequest, Resources_MemoryMbLimit = EXCLUDED.Resources_MemoryMbLimit"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	var query string

	for childIdx, child := range obj.GetConfig().GetEnv() {
		if err := insertIntoDeploymentsContainersEnv(ctx, tx, child, deployments_Id, idx, childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Containers_Env where deployments_Id = $1 AND deployments_Containers_idx = $2 AND idx >= $3"
	_, err = tx.Exec(ctx, query, deployments_Id, idx, len(obj.GetConfig().GetEnv()))
	if err != nil {
		return err
	}
	for childIdx, child := range obj.GetVolumes() {
		if err := insertIntoDeploymentsContainersVolumes(ctx, tx, child, deployments_Id, idx, childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Containers_Volumes where deployments_Id = $1 AND deployments_Containers_idx = $2 AND idx >= $3"
	_, err = tx.Exec(ctx, query, deployments_Id, idx, len(obj.GetVolumes()))
	if err != nil {
		return err
	}
	for childIdx, child := range obj.GetSecrets() {
		if err := insertIntoDeploymentsContainersSecrets(ctx, tx, child, deployments_Id, idx, childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Containers_Secrets where deployments_Id = $1 AND deployments_Containers_idx = $2 AND idx >= $3"
	_, err = tx.Exec(ctx, query, deployments_Id, idx, len(obj.GetSecrets()))
	if err != nil {
		return err
	}
	return nil
}

func insertIntoDeploymentsContainersEnv(ctx context.Context, tx pgx.Tx, obj *storage.ContainerConfig_EnvironmentConfig, deployments_Id string, deployments_Containers_idx int, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		deployments_Containers_idx,
		idx,
		obj.GetKey(),
		obj.GetValue(),
		obj.GetEnvVarSource(),
	}

	finalStr := "INSERT INTO deployments_Containers_Env (deployments_Id, deployments_Containers_idx, idx, Key, Value, EnvVarSource) VALUES($1, $2, $3, $4, $5, $6) ON CONFLICT(deployments_Id, deployments_Containers_idx, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, deployments_Containers_idx = EXCLUDED.deployments_Containers_idx, idx = EXCLUDED.idx, Key = EXCLUDED.Key, Value = EXCLUDED.Value, EnvVarSource = EXCLUDED.EnvVarSource"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	return nil
}

func insertIntoDeploymentsContainersVolumes(ctx context.Context, tx pgx.Tx, obj *storage.Volume, deployments_Id string, deployments_Containers_idx int, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		deployments_Containers_idx,
		idx,
		obj.GetName(),
		obj.GetSource(),
		obj.GetDestination(),
		obj.GetReadOnly(),
		obj.GetType(),
	}

	finalStr := "INSERT INTO deployments_Containers_Volumes (deployments_Id, deployments_Containers_idx, idx, Name, Source, Destination, ReadOnly, Type) VALUES($1, $2, $3, $4, $5, $6, $7, $8) ON CONFLICT(deployments_Id, deployments_Containers_idx, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, deployments_Containers_idx = EXCLUDED.deployments_Containers_idx, idx = EXCLUDED.idx, Name = EXCLUDED.Name, Source = EXCLUDED.Source, Destination = EXCLUDED.Destination, ReadOnly = EXCLUDED.ReadOnly, Type = EXCLUDED.Type"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	return nil
}

func insertIntoDeploymentsContainersSecrets(ctx context.Context, tx pgx.Tx, obj *storage.EmbeddedSecret, deployments_Id string, deployments_Containers_idx int, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		deployments_Containers_idx,
		idx,
		obj.GetName(),
		obj.GetPath(),
	}

	finalStr := "INSERT INTO deployments_Containers_Secrets (deployments_Id, deployments_Containers_idx, idx, Name, Path) VALUES($1, $2, $3, $4, $5) ON CONFLICT(deployments_Id, deployments_Containers_idx, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, deployments_Containers_idx = EXCLUDED.deployments_Containers_idx, idx = EXCLUDED.idx, Name = EXCLUDED.Name, Path = EXCLUDED.Path"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	return nil
}

func insertIntoDeploymentsPorts(ctx context.Context, tx pgx.Tx, obj *storage.PortConfig, deployments_Id string, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		idx,
		obj.GetContainerPort(),
		obj.GetProtocol(),
		obj.GetExposure(),
	}

	finalStr := "INSERT INTO deployments_Ports (deployments_Id, idx, ContainerPort, Protocol, Exposure) VALUES($1, $2, $3, $4, $5) ON CONFLICT(deployments_Id, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, idx = EXCLUDED.idx, ContainerPort = EXCLUDED.ContainerPort, Protocol = EXCLUDED.Protocol, Exposure = EXCLUDED.Exposure"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	var query string

	for childIdx, child := range obj.GetExposureInfos() {
		if err := insertIntoDeploymentsPortsExposureInfos(ctx, tx, child, deployments_Id, idx, childIdx); err != nil {
			return err
		}
	}

	query = "delete from deployments_Ports_ExposureInfos where deployments_Id = $1 AND deployments_Ports_idx = $2 AND idx >= $3"
	_, err = tx.Exec(ctx, query, deployments_Id, idx, len(obj.GetExposureInfos()))
	if err != nil {
		return err
	}
	return nil
}

func insertIntoDeploymentsPortsExposureInfos(ctx context.Context, tx pgx.Tx, obj *storage.PortConfig_ExposureInfo, deployments_Id string, deployments_Ports_idx int, idx int) error {

	values := []interface{}{
		// parent primary keys start
		deployments_Id,
		deployments_Ports_idx,
		idx,
		obj.GetLevel(),
		obj.GetServiceName(),
		obj.GetServicePort(),
		obj.GetNodePort(),
		obj.GetExternalIps(),
		obj.GetExternalHostnames(),
	}

	finalStr := "INSERT INTO deployments_Ports_ExposureInfos (deployments_Id, deployments_Ports_idx, idx, Level, ServiceName, ServicePort, NodePort, ExternalIps, ExternalHostnames) VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9) ON CONFLICT(deployments_Id, deployments_Ports_idx, idx) DO UPDATE SET deployments_Id = EXCLUDED.deployments_Id, deployments_Ports_idx = EXCLUDED.deployments_Ports_idx, idx = EXCLUDED.idx, Level = EXCLUDED.Level, ServiceName = EXCLUDED.ServiceName, ServicePort = EXCLUDED.ServicePort, NodePort = EXCLUDED.NodePort, ExternalIps = EXCLUDED.ExternalIps, ExternalHostnames = EXCLUDED.ExternalHostnames"
	_, err := tx.Exec(ctx, finalStr, values...)
	if err != nil {
		return err
	}

	return nil
}

func (s *storeImpl) copyFromDeployments(ctx context.Context, tx pgx.Tx, objs ...*storage.Deployment) error {

	inputRows := [][]interface{}{}

	var err error

	// This is a copy so first we must delete the rows and re-add them
	// Which is essentially the desired behaviour of an upsert.
	var deletes []string

	copyCols := []string{

		"id",

		"name",

		"type",

		"namespace",

		"namespaceid",

		"orchestratorcomponent",

		"labels",

		"podlabels",

		"created",

		"clusterid",

		"clustername",

		"annotations",

		"priority",

		"imagepullsecrets",

		"serviceaccount",

		"serviceaccountpermissionlevel",

		"riskscore",

		"processtags",

		"serialized",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		serialized, marshalErr := obj.Marshal()
		if marshalErr != nil {
			return marshalErr
		}

		inputRows = append(inputRows, []interface{}{

			obj.GetId(),

			obj.GetName(),

			obj.GetType(),

			obj.GetNamespace(),

			obj.GetNamespaceId(),

			obj.GetOrchestratorComponent(),

			obj.GetLabels(),

			obj.GetPodLabels(),

			pgutils.NilOrTime(obj.GetCreated()),

			obj.GetClusterId(),

			obj.GetClusterName(),

			obj.GetAnnotations(),

			obj.GetPriority(),

			obj.GetImagePullSecrets(),

			obj.GetServiceAccount(),

			obj.GetServiceAccountPermissionLevel(),

			obj.GetRiskScore(),

			obj.GetProcessTags(),

			serialized,
		})

		// Add the id to be deleted.
		deletes = append(deletes, obj.GetId())

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.Exec(ctx, deleteManyStmt, deletes)
			if err != nil {
				return err
			}
			// clear the inserts and vals for the next batch
			deletes = nil

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	for idx, obj := range objs {
		_ = idx // idx may or may not be used depending on how nested we are, so avoid compile-time errors.

		if err = s.copyFromDeploymentsContainers(ctx, tx, obj.GetId(), obj.GetContainers()...); err != nil {
			return err
		}
		if err = s.copyFromDeploymentsPorts(ctx, tx, obj.GetId(), obj.GetPorts()...); err != nil {
			return err
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsContainers(ctx context.Context, tx pgx.Tx, deployments_Id string, objs ...*storage.Container) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"idx",

		"image_id",

		"image_name_registry",

		"image_name_remote",

		"image_name_tag",

		"image_name_fullname",

		"securitycontext_privileged",

		"securitycontext_dropcapabilities",

		"securitycontext_addcapabilities",

		"securitycontext_readonlyrootfilesystem",

		"resources_cpucoresrequest",

		"resources_cpucoreslimit",

		"resources_memorymbrequest",

		"resources_memorymblimit",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			idx,

			obj.GetImage().GetId(),

			obj.GetImage().GetName().GetRegistry(),

			obj.GetImage().GetName().GetRemote(),

			obj.GetImage().GetName().GetTag(),

			obj.GetImage().GetName().GetFullName(),

			obj.GetSecurityContext().GetPrivileged(),

			obj.GetSecurityContext().GetDropCapabilities(),

			obj.GetSecurityContext().GetAddCapabilities(),

			obj.GetSecurityContext().GetReadOnlyRootFilesystem(),

			obj.GetResources().GetCpuCoresRequest(),

			obj.GetResources().GetCpuCoresLimit(),

			obj.GetResources().GetMemoryMbRequest(),

			obj.GetResources().GetMemoryMbLimit(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_containers"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	for idx, obj := range objs {
		_ = idx // idx may or may not be used depending on how nested we are, so avoid compile-time errors.

		if err = s.copyFromDeploymentsContainersEnv(ctx, tx, deployments_Id, idx, obj.GetConfig().GetEnv()...); err != nil {
			return err
		}
		if err = s.copyFromDeploymentsContainersVolumes(ctx, tx, deployments_Id, idx, obj.GetVolumes()...); err != nil {
			return err
		}
		if err = s.copyFromDeploymentsContainersSecrets(ctx, tx, deployments_Id, idx, obj.GetSecrets()...); err != nil {
			return err
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsContainersEnv(ctx context.Context, tx pgx.Tx, deployments_Id string, deployments_Containers_idx int, objs ...*storage.ContainerConfig_EnvironmentConfig) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"deployments_containers_idx",

		"idx",

		"key",

		"value",

		"envvarsource",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			deployments_Containers_idx,

			idx,

			obj.GetKey(),

			obj.GetValue(),

			obj.GetEnvVarSource(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_containers_env"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsContainersVolumes(ctx context.Context, tx pgx.Tx, deployments_Id string, deployments_Containers_idx int, objs ...*storage.Volume) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"deployments_containers_idx",

		"idx",

		"name",

		"source",

		"destination",

		"readonly",

		"type",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			deployments_Containers_idx,

			idx,

			obj.GetName(),

			obj.GetSource(),

			obj.GetDestination(),

			obj.GetReadOnly(),

			obj.GetType(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_containers_volumes"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsContainersSecrets(ctx context.Context, tx pgx.Tx, deployments_Id string, deployments_Containers_idx int, objs ...*storage.EmbeddedSecret) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"deployments_containers_idx",

		"idx",

		"name",

		"path",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			deployments_Containers_idx,

			idx,

			obj.GetName(),

			obj.GetPath(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_containers_secrets"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsPorts(ctx context.Context, tx pgx.Tx, deployments_Id string, objs ...*storage.PortConfig) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"idx",

		"containerport",

		"protocol",

		"exposure",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			idx,

			obj.GetContainerPort(),

			obj.GetProtocol(),

			obj.GetExposure(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_ports"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	for idx, obj := range objs {
		_ = idx // idx may or may not be used depending on how nested we are, so avoid compile-time errors.

		if err = s.copyFromDeploymentsPortsExposureInfos(ctx, tx, deployments_Id, idx, obj.GetExposureInfos()...); err != nil {
			return err
		}
	}

	return err
}

func (s *storeImpl) copyFromDeploymentsPortsExposureInfos(ctx context.Context, tx pgx.Tx, deployments_Id string, deployments_Ports_idx int, objs ...*storage.PortConfig_ExposureInfo) error {

	inputRows := [][]interface{}{}

	var err error

	copyCols := []string{

		"deployments_id",

		"deployments_ports_idx",

		"idx",

		"level",

		"servicename",

		"serviceport",

		"nodeport",

		"externalips",

		"externalhostnames",
	}

	for idx, obj := range objs {
		// Todo: ROX-9499 Figure out how to more cleanly template around this issue.
		log.Debugf("This is here for now because there is an issue with pods_TerminatedInstances where the obj in the loop is not used as it only consists of the parent id and the idx.  Putting this here as a stop gap to simply use the object.  %s", obj)

		inputRows = append(inputRows, []interface{}{

			deployments_Id,

			deployments_Ports_idx,

			idx,

			obj.GetLevel(),

			obj.GetServiceName(),

			obj.GetServicePort(),

			obj.GetNodePort(),

			obj.GetExternalIps(),

			obj.GetExternalHostnames(),
		})

		// if we hit our batch size we need to push the data
		if (idx+1)%batchSize == 0 || idx == len(objs)-1 {
			// copy does not upsert so have to delete first.  parent deletion cascades so only need to
			// delete for the top level parent

			_, err = tx.CopyFrom(ctx, pgx.Identifier{"deployments_ports_exposureinfos"}, copyCols, pgx.CopyFromRows(inputRows))

			if err != nil {
				return err
			}

			// clear the input rows for the next batch
			inputRows = inputRows[:0]
		}
	}

	return err
}

func (s *storeImpl) copyFrom(ctx context.Context, objs ...*storage.Deployment) error {
	conn, release, err := s.acquireConn(ctx, ops.Get, "Deployment")
	if err != nil {
		return err
	}
	defer release()

	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}

	if err := s.copyFromDeployments(ctx, tx, objs...); err != nil {
		if err := tx.Rollback(ctx); err != nil {
			return err
		}
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return nil
}

func (s *storeImpl) upsert(ctx context.Context, objs ...*storage.Deployment) error {
	conn, release, err := s.acquireConn(ctx, ops.Get, "Deployment")
	if err != nil {
		return err
	}
	defer release()

	for _, obj := range objs {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}

		if err := insertIntoDeployments(ctx, tx, obj); err != nil {
			if err := tx.Rollback(ctx); err != nil {
				return err
			}
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *storeImpl) Upsert(ctx context.Context, obj *storage.Deployment) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Upsert, "Deployment")

	scopeChecker := sac.GlobalAccessScopeChecker(ctx).AccessMode(storage.Access_READ_WRITE_ACCESS).Resource(targetResource).
		ClusterID(obj.GetClusterId()).Namespace(obj.GetNamespace())
	if ok, err := scopeChecker.Allowed(ctx); err != nil {
		return err
	} else if !ok {
		return sac.ErrResourceAccessDenied
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	return s.upsert(ctx, obj)
}

func (s *storeImpl) UpsertMany(ctx context.Context, objs []*storage.Deployment) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.UpdateMany, "Deployment")

	scopeChecker := sac.GlobalAccessScopeChecker(ctx).AccessMode(storage.Access_READ_WRITE_ACCESS).Resource(targetResource)
	if ok, err := scopeChecker.Allowed(ctx); err != nil {
		return err
	} else if !ok {
		var deniedIds []string
		for _, obj := range objs {
			subScopeChecker := scopeChecker.ClusterID(obj.GetClusterId()).Namespace(obj.GetNamespace())
			if ok, err := subScopeChecker.Allowed(ctx); err != nil {
				return err
			} else if !ok {
				deniedIds = append(deniedIds, obj.GetId())
			}
		}
		if len(deniedIds) != 0 {
			return errors.Wrapf(sac.ErrResourceAccessDenied, "modifying deployments with IDs [%s] was denied", strings.Join(deniedIds, ", "))
		}
	}

	s.mutex.Lock()
	defer s.mutex.Unlock()

	if len(objs) < batchAfter {
		return s.upsert(ctx, objs...)
	} else {
		return s.copyFrom(ctx, objs...)
	}
}

// Count returns the number of objects in the store
func (s *storeImpl) Count(ctx context.Context) (int, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Count, "Deployment")

	var sacQueryFilter *v1.Query

	scopeChecker := sac.GlobalAccessScopeChecker(ctx)
	scopeTree, err := scopeChecker.EffectiveAccessScope(permissions.View(targetResource))
	if err != nil {
		return 0, err
	}
	sacQueryFilter, err = sac.BuildClusterNamespaceLevelSACQueryFilter(scopeTree)

	if err != nil {
		return 0, err
	}

	return postgres.RunCountRequestForSchema(schema, sacQueryFilter, s.db)
}

// Exists returns if the id exists in the store
func (s *storeImpl) Exists(ctx context.Context, id string) (bool, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Exists, "Deployment")

	var sacQueryFilter *v1.Query
	scopeChecker := sac.GlobalAccessScopeChecker(ctx)
	scopeTree, err := scopeChecker.EffectiveAccessScope(permissions.View(targetResource))
	if err != nil {
		return false, err
	}
	sacQueryFilter, err = sac.BuildClusterNamespaceLevelSACQueryFilter(scopeTree)
	if err != nil {
		return false, err
	}

	q := search.ConjunctionQuery(
		sacQueryFilter,
		search.NewQueryBuilder().AddDocIDs(id).ProtoQuery(),
	)

	count, err := postgres.RunCountRequestForSchema(schema, q, s.db)
	return count == 1, err
}

// Get returns the object, if it exists from the store
func (s *storeImpl) Get(ctx context.Context, id string) (*storage.Deployment, bool, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Get, "Deployment")

	conn, release, err := s.acquireConn(ctx, ops.Get, "Deployment")
	if err != nil {
		return nil, false, err
	}
	defer release()

	row := conn.QueryRow(ctx, getStmt, id)
	var data []byte
	if err := row.Scan(&data); err != nil {
		return nil, false, pgutils.ErrNilIfNoRows(err)
	}

	var msg storage.Deployment
	if err := proto.Unmarshal(data, &msg); err != nil {
		return nil, false, err
	}
	return &msg, true, nil
}

func (s *storeImpl) acquireConn(ctx context.Context, op ops.Op, typ string) (*pgxpool.Conn, func(), error) {
	defer metrics.SetAcquireDBConnDuration(time.Now(), op, typ)
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, nil, err
	}
	return conn, conn.Release, nil
}

// Delete removes the specified ID from the store
func (s *storeImpl) Delete(ctx context.Context, id string) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.Remove, "Deployment")

	conn, release, err := s.acquireConn(ctx, ops.Remove, "Deployment")
	if err != nil {
		return err
	}
	defer release()

	if _, err := conn.Exec(ctx, deleteStmt, id); err != nil {
		return err
	}
	return nil
}

// GetIDs returns all the IDs for the store
func (s *storeImpl) GetIDs(ctx context.Context) ([]string, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.GetAll, "storage.DeploymentIDs")
	var sacQueryFilter *v1.Query

	scopeChecker := sac.GlobalAccessScopeChecker(ctx)
	scopeTree, err := scopeChecker.EffectiveAccessScope(permissions.View(targetResource))
	if err != nil {
		return nil, err
	}
	sacQueryFilter, err = sac.BuildClusterNamespaceLevelSACQueryFilter(scopeTree)
	if err != nil {
		return nil, err
	}
	result, err := postgres.RunSearchRequestForSchema(schema, sacQueryFilter, s.db)
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(result))
	for _, entry := range result {
		ids = append(ids, entry.ID)
	}

	return ids, nil
}

// GetMany returns the objects specified by the IDs or the index in the missing indices slice
func (s *storeImpl) GetMany(ctx context.Context, ids []string) ([]*storage.Deployment, []int, error) {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.GetMany, "Deployment")

	conn, release, err := s.acquireConn(ctx, ops.GetMany, "Deployment")
	if err != nil {
		return nil, nil, err
	}
	defer release()

	rows, err := conn.Query(ctx, getManyStmt, ids)
	if err != nil {
		if err == pgx.ErrNoRows {
			missingIndices := make([]int, 0, len(ids))
			for i := range ids {
				missingIndices = append(missingIndices, i)
			}
			return nil, missingIndices, nil
		}
		return nil, nil, err
	}
	defer rows.Close()
	resultsByID := make(map[string]*storage.Deployment)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, nil, err
		}
		msg := &storage.Deployment{}
		if err := proto.Unmarshal(data, msg); err != nil {
			return nil, nil, err
		}
		resultsByID[msg.GetId()] = msg
	}
	missingIndices := make([]int, 0, len(ids)-len(resultsByID))
	// It is important that the elems are populated in the same order as the input ids
	// slice, since some calling code relies on that to maintain order.
	elems := make([]*storage.Deployment, 0, len(resultsByID))
	for i, id := range ids {
		if result, ok := resultsByID[id]; !ok {
			missingIndices = append(missingIndices, i)
		} else {
			elems = append(elems, result)
		}
	}
	return elems, missingIndices, nil
}

// Delete removes the specified IDs from the store
func (s *storeImpl) DeleteMany(ctx context.Context, ids []string) error {
	defer metrics.SetPostgresOperationDurationTime(time.Now(), ops.RemoveMany, "Deployment")

	conn, release, err := s.acquireConn(ctx, ops.RemoveMany, "Deployment")
	if err != nil {
		return err
	}
	defer release()
	if _, err := conn.Exec(ctx, deleteManyStmt, ids); err != nil {
		return err
	}
	return nil
}

// Walk iterates over all of the objects in the store and applies the closure
func (s *storeImpl) Walk(ctx context.Context, fn func(obj *storage.Deployment) error) error {
	rows, err := s.db.Query(ctx, walkStmt)
	if err != nil {
		return pgutils.ErrNilIfNoRows(err)
	}
	defer rows.Close()
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return err
		}
		var msg storage.Deployment
		if err := proto.Unmarshal(data, &msg); err != nil {
			return err
		}
		if err := fn(&msg); err != nil {
			return err
		}
	}
	return nil
}

//// Used for testing

func dropTableDeployments(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments CASCADE")
	dropTableDeploymentsContainers(ctx, db)
	dropTableDeploymentsPorts(ctx, db)

}

func dropTableDeploymentsContainers(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Containers CASCADE")
	dropTableDeploymentsContainersEnv(ctx, db)
	dropTableDeploymentsContainersVolumes(ctx, db)
	dropTableDeploymentsContainersSecrets(ctx, db)

}

func dropTableDeploymentsContainersEnv(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Containers_Env CASCADE")

}

func dropTableDeploymentsContainersVolumes(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Containers_Volumes CASCADE")

}

func dropTableDeploymentsContainersSecrets(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Containers_Secrets CASCADE")

}

func dropTableDeploymentsPorts(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Ports CASCADE")
	dropTableDeploymentsPortsExposureInfos(ctx, db)

}

func dropTableDeploymentsPortsExposureInfos(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, "DROP TABLE IF EXISTS deployments_Ports_ExposureInfos CASCADE")

}

func Destroy(ctx context.Context, db *pgxpool.Pool) {
	dropTableDeployments(ctx, db)
}

//// Stubs for satisfying legacy interfaces

// AckKeysIndexed acknowledges the passed keys were indexed
func (s *storeImpl) AckKeysIndexed(ctx context.Context, keys ...string) error {
	return nil
}

// GetKeysToIndex returns the keys that need to be indexed
func (s *storeImpl) GetKeysToIndex(ctx context.Context) ([]string, error) {
	return nil, nil
}
