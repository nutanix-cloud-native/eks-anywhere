package docker

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"regexp"
	"time"

	etcdv1alpha3 "github.com/mrajashree/etcdadm-controller/api/v1alpha3"
	"sigs.k8s.io/cluster-api/api/v1alpha3"
	kubeadmnv1alpha3 "sigs.k8s.io/cluster-api/controlplane/kubeadm/api/v1alpha3"

	"github.com/aws/eks-anywhere/pkg/api/v1alpha1"
	"github.com/aws/eks-anywhere/pkg/bootstrapper"
	"github.com/aws/eks-anywhere/pkg/cluster"
	"github.com/aws/eks-anywhere/pkg/clusterapi"
	"github.com/aws/eks-anywhere/pkg/constants"
	"github.com/aws/eks-anywhere/pkg/executables"
	"github.com/aws/eks-anywhere/pkg/logger"
	"github.com/aws/eks-anywhere/pkg/providers"
	"github.com/aws/eks-anywhere/pkg/providers/common"
	"github.com/aws/eks-anywhere/pkg/templater"
	"github.com/aws/eks-anywhere/pkg/types"
	releasev1alpha1 "github.com/aws/eks-anywhere/release/api/v1alpha1"
)

const (
	githubTokenEnvVar = "GITHUB_TOKEN"
)

//go:embed config/template-cp.yaml
var defaultCAPIConfigCP string

//go:embed config/template-md.yaml
var defaultCAPIConfigMD string

var eksaDockerResourceType = fmt.Sprintf("dockerdatacenterconfigs.%s", v1alpha1.GroupVersion.Group)

type ProviderClient interface {
	GetDockerLBPort(ctx context.Context, clusterName string) (port string, err error)
}

type provider struct {
	docker                ProviderClient
	datacenterConfig      *v1alpha1.DockerDatacenterConfig
	providerKubectlClient ProviderKubectlClient
	templateBuilder       *DockerTemplateBuilder
}

type ProviderKubectlClient interface {
	GetEksaCluster(ctx context.Context, cluster *types.Cluster, clusterName string) (*v1alpha1.Cluster, error)
	GetKubeadmControlPlane(ctx context.Context, cluster *types.Cluster, clusterName string, opts ...executables.KubectlOpt) (*kubeadmnv1alpha3.KubeadmControlPlane, error)
	GetMachineDeployment(ctx context.Context, cluster *types.Cluster, clusterName string, opts ...executables.KubectlOpt) (*v1alpha3.MachineDeployment, error)
	GetEtcdadmCluster(ctx context.Context, cluster *types.Cluster, clusterName string, opts ...executables.KubectlOpt) (*etcdv1alpha3.EtcdadmCluster, error)
	UpdateAnnotation(ctx context.Context, resourceType, objectName string, annotations map[string]string, opts ...executables.KubectlOpt) error
}

func NewProvider(providerConfig *v1alpha1.DockerDatacenterConfig, docker ProviderClient, providerKubectlClient ProviderKubectlClient, now types.NowFunc) providers.Provider {
	return &provider{
		docker:                docker,
		datacenterConfig:      providerConfig,
		providerKubectlClient: providerKubectlClient,
		templateBuilder: &DockerTemplateBuilder{
			now: now,
		},
	}
}

func (p *provider) BootstrapClusterOpts() ([]bootstrapper.BootstrapClusterOption, error) {
	return []bootstrapper.BootstrapClusterOption{bootstrapper.WithExtraDockerMounts()}, nil
}

func (p *provider) BootstrapSetup(ctx context.Context, clusterConfig *v1alpha1.Cluster, cluster *types.Cluster) error {
	return nil
}

func (p *provider) Name() string {
	return constants.DockerProviderName
}

func (p *provider) DatacenterResourceType() string {
	return eksaDockerResourceType
}

func (p *provider) MachineResourceType() string {
	return ""
}

func (p *provider) SetupAndValidateCreateCluster(ctx context.Context, clusterSpec *cluster.Spec) error {
	logger.Info("Warning: The docker infrastructure provider is meant for local development and testing only")
	if clusterSpec.Spec.ControlPlaneConfiguration.Endpoint != nil && clusterSpec.Spec.ControlPlaneConfiguration.Endpoint.Host != "" {
		return fmt.Errorf("specifying endpoint host configuration in Cluster is not supported")
	}
	return nil
}

func (p *provider) SetupAndValidateDeleteCluster(ctx context.Context) error {
	return nil
}

func (p *provider) SetupAndValidateUpgradeCluster(ctx context.Context, _ *cluster.Spec) error {
	return nil
}

func (p *provider) UpdateSecrets(ctx context.Context, cluster *types.Cluster) error {
	// Not implemented
	return nil
}

func NewDockerTemplateBuilder(now types.NowFunc) providers.TemplateBuilder {
	return &DockerTemplateBuilder{
		now: now,
	}
}

type DockerTemplateBuilder struct {
	now types.NowFunc
}

func (d *DockerTemplateBuilder) WorkerMachineTemplateName(clusterName string) string {
	t := d.now().UnixNano() / int64(time.Millisecond)
	return fmt.Sprintf("%s-worker-node-template-%d", clusterName, t)
}

func (d *DockerTemplateBuilder) CPMachineTemplateName(clusterName string) string {
	t := d.now().UnixNano() / int64(time.Millisecond)
	return fmt.Sprintf("%s-control-plane-template-%d", clusterName, t)
}

func (d *DockerTemplateBuilder) EtcdMachineTemplateName(clusterName string) string {
	t := d.now().UnixNano() / int64(time.Millisecond)
	return fmt.Sprintf("%s-etcd-template-%d", clusterName, t)
}

func (d *DockerTemplateBuilder) GenerateCAPISpecControlPlane(clusterSpec *cluster.Spec, buildOptions ...providers.BuildMapOption) (content []byte, err error) {
	values := buildTemplateMapCP(clusterSpec)
	for _, buildOption := range buildOptions {
		buildOption(values)
	}

	bytes, err := templater.Execute(defaultCAPIConfigCP, values)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func (d *DockerTemplateBuilder) GenerateCAPISpecWorkers(clusterSpec *cluster.Spec, buildOptions ...providers.BuildMapOption) (content []byte, err error) {
	values := buildTemplateMapMD(clusterSpec)
	for _, buildOption := range buildOptions {
		buildOption(values)
	}

	bytes, err := templater.Execute(defaultCAPIConfigMD, values)
	if err != nil {
		return nil, err
	}

	return bytes, nil
}

func buildTemplateMapCP(clusterSpec *cluster.Spec) map[string]interface{} {
	bundle := clusterSpec.VersionsBundle

	values := map[string]interface{}{
		"clusterName":            clusterSpec.Name,
		"control_plane_replicas": clusterSpec.Spec.ControlPlaneConfiguration.Count,
		"kubernetesRepository":   bundle.KubeDistro.Kubernetes.Repository,
		"kubernetesVersion":      bundle.KubeDistro.Kubernetes.Tag,
		"etcdRepository":         bundle.KubeDistro.Etcd.Repository,
		"etcdVersion":            bundle.KubeDistro.Etcd.Tag,
		"corednsRepository":      bundle.KubeDistro.CoreDNS.Repository,
		"corednsVersion":         bundle.KubeDistro.CoreDNS.Tag,
		"kindNodeImage":          bundle.EksD.KindNode.VersionedImage(),
		"apiserverExtraArgs":     clusterapi.OIDCToExtraArgs(clusterSpec.OIDCConfig).Append(clusterapi.AwsIamAuthExtraArgs(clusterSpec.AWSIamConfig)).ToPartialYaml(),
		"externalEtcdVersion":    bundle.KubeDistro.EtcdVersion,
		"eksaSystemNamespace":    constants.EksaSystemNamespace,
		"auditPolicy":            common.GetAuditPolicy(),
		"podCidrs":               clusterSpec.Spec.ClusterNetwork.Pods.CidrBlocks,
		"serviceCidrs":           clusterSpec.Spec.ClusterNetwork.Services.CidrBlocks,
	}

	if clusterSpec.Spec.ExternalEtcdConfiguration != nil {
		values["externalEtcd"] = true
		values["externalEtcdReplicas"] = clusterSpec.Spec.ExternalEtcdConfiguration.Count
	}
	if clusterSpec.AWSIamConfig != nil {
		values["awsIamAuth"] = true
	}
	return values
}

func buildTemplateMapMD(clusterSpec *cluster.Spec) map[string]interface{} {
	bundle := clusterSpec.VersionsBundle

	values := map[string]interface{}{
		"clusterName":         clusterSpec.Name,
		"worker_replicas":     clusterSpec.Spec.WorkerNodeGroupConfigurations[0].Count,
		"kubernetesVersion":   bundle.KubeDistro.Kubernetes.Tag,
		"kindNodeImage":       bundle.EksD.KindNode.VersionedImage(),
		"eksaSystemNamespace": constants.EksaSystemNamespace,
	}
	return values
}

func NeedsNewControlPlaneTemplate(oldSpec, newSpec *cluster.Spec) bool {
	return (oldSpec.Cluster.Spec.KubernetesVersion != newSpec.Cluster.Spec.KubernetesVersion) || (oldSpec.Bundles.Spec.Number != newSpec.Bundles.Spec.Number)
}

func NeedsNewWorkloadTemplate(oldC, newC *v1alpha1.Cluster) bool {
	return oldC.Spec.KubernetesVersion != newC.Spec.KubernetesVersion
}

func NeedsNewEtcdTemplate(oldC, newC *v1alpha1.Cluster) bool {
	return oldC.Spec.KubernetesVersion != newC.Spec.KubernetesVersion
}

func (p *provider) generateCAPISpecForUpgrade(ctx context.Context, bootstrapCluster, workloadCluster *types.Cluster, currentSpec, newClusterSpec *cluster.Spec) (controlPlaneSpec, workersSpec []byte, err error) {
	clusterName := newClusterSpec.ObjectMeta.Name
	var controlPlaneTemplateName, workloadTemplateName, etcdTemplateName string
	var needsNewEtcdTemplate bool

	c, err := p.providerKubectlClient.GetEksaCluster(ctx, workloadCluster, newClusterSpec.Name)
	if err != nil {
		return nil, nil, err
	}

	needsNewControlPlaneTemplate := NeedsNewControlPlaneTemplate(currentSpec, newClusterSpec)
	if !needsNewControlPlaneTemplate {
		cp, err := p.providerKubectlClient.GetKubeadmControlPlane(ctx, workloadCluster, workloadCluster.Name, executables.WithCluster(bootstrapCluster), executables.WithNamespace(constants.EksaSystemNamespace))
		if err != nil {
			return nil, nil, err
		}
		controlPlaneTemplateName = cp.Spec.InfrastructureTemplate.Name
	} else {
		controlPlaneTemplateName = p.templateBuilder.CPMachineTemplateName(clusterName)
	}

	needsNewWorkloadTemplate := NeedsNewWorkloadTemplate(c, newClusterSpec.Cluster)
	if !needsNewWorkloadTemplate {
		md, err := p.providerKubectlClient.GetMachineDeployment(ctx, workloadCluster, newClusterSpec.Name, executables.WithCluster(bootstrapCluster), executables.WithNamespace(constants.EksaSystemNamespace))
		if err != nil {
			return nil, nil, err
		}
		workloadTemplateName = md.Spec.Template.Spec.InfrastructureRef.Name
	} else {
		workloadTemplateName = p.templateBuilder.WorkerMachineTemplateName(clusterName)
	}

	if newClusterSpec.Spec.ExternalEtcdConfiguration != nil {
		// TODO: replace controlPlaneMachineConfig with etcdMachineConfig once available in final GA spec
		needsNewEtcdTemplate = NeedsNewEtcdTemplate(c, newClusterSpec.Cluster)
		if !needsNewEtcdTemplate {
			etcdadmCluster, err := p.providerKubectlClient.GetEtcdadmCluster(ctx, workloadCluster, newClusterSpec.Name, executables.WithCluster(bootstrapCluster), executables.WithNamespace(constants.EksaSystemNamespace))
			if err != nil {
				return nil, nil, err
			}
			etcdTemplateName = etcdadmCluster.Spec.InfrastructureTemplate.Name
		} else {
			/* During a cluster upgrade, etcd machines need to be upgraded first, so that the etcd machines with new spec get created and can be used by controlplane machines
			as etcd endpoints. KCP rollout should not start until then. As a temporary solution in the absence of static etcd endpoints, we annotate the etcd cluster as "upgrading",
			so that KCP checks this annotation and does not proceed if etcd cluster is upgrading. The etcdadm controller removes this annotation once the etcd upgrade is complete.
			*/
			err = p.providerKubectlClient.UpdateAnnotation(ctx, "etcdadmcluster", fmt.Sprintf("%s-etcd", newClusterSpec.Name),
				map[string]string{etcdv1alpha3.UpgradeInProgressAnnotation: "true"},
				executables.WithCluster(bootstrapCluster),
				executables.WithNamespace(constants.EksaSystemNamespace))
			if err != nil {
				return nil, nil, err
			}
			etcdTemplateName = p.templateBuilder.EtcdMachineTemplateName(clusterName)
		}
	}

	cpOpt := func(values map[string]interface{}) {
		values["controlPlaneTemplateName"] = controlPlaneTemplateName
		values["etcdTemplateName"] = etcdTemplateName
	}
	controlPlaneSpec, err = p.templateBuilder.GenerateCAPISpecControlPlane(newClusterSpec, cpOpt)
	if err != nil {
		return nil, nil, err
	}

	workersOpts := func(values map[string]interface{}) {
		values["workloadTemplateName"] = workloadTemplateName
	}
	workersSpec, err = p.templateBuilder.GenerateCAPISpecWorkers(newClusterSpec, workersOpts)
	if err != nil {
		return nil, nil, err
	}
	return controlPlaneSpec, workersSpec, nil
}

func (p *provider) generateCAPISpecForCreate(ctx context.Context, cluster *types.Cluster, clusterSpec *cluster.Spec) (controlPlaneSpec, workersSpec []byte, err error) {
	clusterName := clusterSpec.ObjectMeta.Name

	cpOpt := func(values map[string]interface{}) {
		values["controlPlaneTemplateName"] = p.templateBuilder.CPMachineTemplateName(clusterName)
		values["etcdTemplateName"] = p.templateBuilder.EtcdMachineTemplateName(clusterName)
	}
	controlPlaneSpec, err = p.templateBuilder.GenerateCAPISpecControlPlane(clusterSpec, cpOpt)
	if err != nil {
		return nil, nil, err
	}
	workersOpts := func(values map[string]interface{}) {
		values["workloadTemplateName"] = p.templateBuilder.WorkerMachineTemplateName(clusterName)
	}
	workersSpec, err = p.templateBuilder.GenerateCAPISpecWorkers(clusterSpec, workersOpts)
	if err != nil {
		return nil, nil, err
	}
	return controlPlaneSpec, workersSpec, nil
}

func (p *provider) GenerateCAPISpecForCreate(ctx context.Context, cluster *types.Cluster, clusterSpec *cluster.Spec) (controlPlaneSpec, workersSpec []byte, err error) {
	controlPlaneSpec, workersSpec, err = p.generateCAPISpecForCreate(ctx, cluster, clusterSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating cluster api spec contents: %v", err)
	}
	return controlPlaneSpec, workersSpec, nil
}

func (p *provider) GenerateCAPISpecForUpgrade(ctx context.Context, bootstrapCluster, workloadCluster *types.Cluster, currentSpec, newClusterSpec *cluster.Spec) (controlPlaneSpec, workersSpec []byte, err error) {
	controlPlaneSpec, workersSpec, err = p.generateCAPISpecForUpgrade(ctx, bootstrapCluster, workloadCluster, currentSpec, newClusterSpec)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating cluster api spec contents: %v", err)
	}
	return controlPlaneSpec, workersSpec, nil
}

func (p *provider) GenerateStorageClass() []byte {
	return nil
}

func (p *provider) GenerateMHC() ([]byte, error) {
	return []byte{}, nil
}

func (p *provider) UpdateKubeConfig(content *[]byte, clusterName string) error {
	// The Docker provider is for testing only. We don't want to change the interface just for the test
	ctx := context.Background()
	if port, err := p.docker.GetDockerLBPort(ctx, clusterName); err != nil {
		return err
	} else {
		getUpdatedKubeConfigContent(content, port)
		return nil
	}
}

// this is required for docker provider
func getUpdatedKubeConfigContent(content *[]byte, dockerLbPort string) {
	mc := regexp.MustCompile("server:.*")
	updatedConfig := mc.ReplaceAllString(string(*content), fmt.Sprintf("server: https://127.0.0.1:%s", dockerLbPort))
	mc = regexp.MustCompile("certificate-authority-data:.*")
	updatedConfig = mc.ReplaceAllString(updatedConfig, "insecure-skip-tls-verify: true")
	updatedContentByte := []byte(updatedConfig)
	*content = updatedContentByte
}

func (p *provider) CleanupProviderInfrastructure(_ context.Context) error {
	return nil
}

func (p *provider) Version(clusterSpec *cluster.Spec) string {
	return clusterSpec.VersionsBundle.Docker.Version
}

func (p *provider) EnvMap() (map[string]string, error) {
	envMap := make(map[string]string)
	if env, ok := os.LookupEnv(githubTokenEnvVar); ok && len(env) > 0 {
		envMap[githubTokenEnvVar] = env
	}
	return envMap, nil
}

func (p *provider) GetDeployments() map[string][]string {
	return map[string][]string{
		"capd-system": {"capd-controller-manager"},
	}
}

func (p *provider) GetInfrastructureBundle(clusterSpec *cluster.Spec) *types.InfrastructureBundle {
	bundle := clusterSpec.VersionsBundle
	folderName := fmt.Sprintf("infrastructure-docker/%s/", bundle.Docker.Version)

	infraBundle := types.InfrastructureBundle{
		FolderName: folderName,
		Manifests: []releasev1alpha1.Manifest{
			bundle.Docker.Components,
			bundle.Docker.Metadata,
			bundle.Docker.ClusterTemplate,
		},
	}

	return &infraBundle
}

func (p *provider) DatacenterConfig() providers.DatacenterConfig {
	return p.datacenterConfig
}

func (p *provider) MachineConfigs() []providers.MachineConfig {
	return nil
}

func (p *provider) ValidateNewSpec(_ context.Context, _ *types.Cluster, _ *cluster.Spec) error {
	return nil
}

func (p *provider) ChangeDiff(currentSpec, newSpec *cluster.Spec) *types.ComponentChangeDiff {
	if currentSpec.VersionsBundle.Docker.Version == newSpec.VersionsBundle.Docker.Version {
		return nil
	}

	return &types.ComponentChangeDiff{
		ComponentName: constants.DockerProviderName,
		NewVersion:    newSpec.VersionsBundle.Docker.Version,
		OldVersion:    currentSpec.VersionsBundle.Docker.Version,
	}
}
