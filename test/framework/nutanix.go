package framework

import (
	"testing"

	"github.com/aws/eks-anywhere/internal/pkg/api"
)

const (
	nutanixEndpoint = "T_NUTANIX_ENDPOINT"
	nutanixPort     = "T_NUTANIX_PORT"
	nutanixUser     = "T_NUTANIX_USER"
	nutanixPwd      = "T_NUTANIX_PASSWORD"
	nutanixInsecure = "T_NUTANIX_INSECURE"

	nutanixMachineBootType       = "T_NUTANIX_MACHINE_BOOT_TYPE"
	nutanixMachineMemorySize     = "T_NUTANIX_MACHINE_MEMORY_SIZE"
	nutanixSystemDiskSize        = "T_NUTANIX_SYSTEMDISK_SIZE"
	nutanixMachineVCPUsPerSocket = "T_NUTANIX_MACHINE_VCPU_PER_SOCKET"
	nutanixMachineVCPUSocket     = "T_NUTANIX_MACHINE_VCPU_SOCKET"

	nutanixMachineTemplateImageName = "T_NUTANIX_MACHINE_TEMPLATE_IMAGE_NAME"
	nutanixPrismElementClusterName  = "T_NUTANIX_PRISM_ELEMENT_CLUSTER_NAME"
	nutanixSSHAuthorizedKey         = "T_NUTANIX_SSH_AUTHORIZED_KEY"
	nutanixSubnetName               = "T_NUTANIX_SUBNET_NAME"
)

var requiredNutanixEnvVars = []string{
	nutanixEndpoint,
	nutanixPort,
	nutanixUser,
	nutanixPwd,
	nutanixInsecure,

	nutanixMachineBootType,
	nutanixMachineMemorySize,
	nutanixSystemDiskSize,
	nutanixMachineVCPUsPerSocket,
	nutanixMachineVCPUSocket,
	nutanixMachineTemplateImageName,

	nutanixPrismElementClusterName,
	nutanixSSHAuthorizedKey,
	nutanixSubnetName,
}

type Nutanix struct {
	t              *testing.T
	fillers        []api.NutanixFiller
	clusterFillers []api.ClusterFiller
	cpCidr         string
	podCidr        string
}

type NutanixOpt func(*Nutanix)

func NewNutanix(t *testing.T, opts ...NutanixOpt) *Nutanix {
	checkRequiredEnvVars(t, requiredNutanixEnvVars)
	nutanixProvider := &Nutanix{
		t: t,
		fillers: []api.NutanixFiller{
			api.WithNutanixStringFromEnvVar(nutanixEndpoint, api.WithNutanixEndpoint),
			api.WithNutanixIntFromEnvVar(nutanixPort, api.WithNutanixPort),
			api.WithNutanixStringFromEnvVar(nutanixUser, api.WithNutanixUser),
			api.WithNutanixStringFromEnvVar(nutanixPwd, api.WithNutanixPwd),
			api.WithNutanixBoolFromEnvVar(nutanixInsecure, api.WithNutanixInsure),

			// api.WithNutanixStringFromEnvVar(nutanixMachineBootType, api.WithNutanixMachineBootType),
			api.WithNutanixStringFromEnvVar(nutanixMachineMemorySize, api.WithNutanixMachineMemorySize),
			api.WithNutanixStringFromEnvVar(nutanixSystemDiskSize, api.WithNutanixMachineSystemDiskSize),
			api.WithNutanixInt32FromEnvVar(nutanixMachineVCPUsPerSocket, api.WithNutanixMachineVCPUsPerSocket),
			api.WithNutanixInt32FromEnvVar(nutanixMachineVCPUSocket, api.WithNutanixMachineVCPUSocket),
			api.WithNutanixStringFromEnvVar(nutanixMachineTemplateImageName, api.WithNutanixMachineTemplateImageName),

			api.WithNutanixStringFromEnvVar(nutanixPrismElementClusterName, api.WithNutanixPrismElementClusterName),
			api.WithNutanixStringFromEnvVar(nutanixSSHAuthorizedKey, api.WithNutanixSSHAuthorizedKey),
			api.WithNutanixStringFromEnvVar(nutanixSubnetName, api.WithNutanixSubnetName),
		},
	}

	// s.cpCidr = os.Getenv(nutanixControlPlaneCidr)
	// s.podCidr = os.Getenv(nutanixPodCidr)

	for _, opt := range opts {
		opt(nutanixProvider)
	}

	return nutanixProvider
}

func (s *Nutanix) Name() string {
	return "nutanix"
}

func (s *Nutanix) Setup() {}

func (s *Nutanix) CustomizeProviderConfig(file string) []byte {
	return s.customizeProviderConfig(file, s.fillers...)
}

func (s *Nutanix) ClusterConfigFillers() []api.ClusterFiller {
	// ip, err := GenerateUniqueIp(s.cpCidr)
	// if err != nil {
	// 	s.t.Fatalf("failed to generate control plane ip for nutanix [cidr=%s]: %v", s.cpCidr, err)
	// }
	// s.clusterFillers = append(s.clusterFillers, api.WithControlPlaneEndpointIP(ip))

	// if s.podCidr != "" {
	// 	s.clusterFillers = append(s.clusterFillers, api.WithPodCidr(s.podCidr))
	// }

	return s.clusterFillers
}

func (s *Nutanix) customizeProviderConfig(file string, fillers ...api.NutanixFiller) []byte {
	providerOutput, err := api.AutoFillNutanixProvider(file, fillers...)
	if err != nil {
		s.t.Fatalf("failed to customize provider config from file: %v", err)
	}
	return providerOutput
}

func WithNutanixUbuntu121() NutanixOpt {
	return func(v *Nutanix) {
		v.fillers = append(v.fillers,
			api.WithNutanixInt32FromEnvVar(nutanixMachineVCPUsPerSocket, api.WithNutanixMachineVCPUsPerSocket),
		)
	}
}
