package autoscale

// import (
// 	"time"

// 	"github.com/go-logr/logr"
// 	operatorv1alpha1 "github.com/pingcap/tidb-operator/pkg/apis/pingcap/v1alpha1"
// 	cloudv1 "github.com/tidbcloud/infra-api/api/v1"
// 	gitopssdk "github.com/tidbcloud/infra-api/gitops-sdk"
// 	"github.com/tidbcloud/infra-api/pkg/gitops"
// 	gitopsv2 "github.com/tidbcloud/infra-api/pkg/gitops-v2"
// 	"sigs.k8s.io/controller-runtime/pkg/client"
// 	// "github.com/tidbcloud/eks-provider/api/v1alpha1"
// 	// "github.com/tidbcloud/eks-provider/pkg/account"
// 	// "github.com/tidbcloud/eks-provider/pkg/config"
// 	// "github.com/tidbcloud/eks-provider/pkg/k8s"
// 	// "github.com/tidbcloud/eks-provider/pkg/namer"
// )

// type Context struct {
// 	// Transient during a deployment
// 	// remote      k8s.ClientGetter
// 	tidbcluster *operatorv1alpha1.TidbCluster
// 	// namer       *namer.Namer
// 	pdClient gitopssdk.DynamicPDClient

// 	// Injected from the caller
// 	Local   client.Client
// 	Cluster *cloudv1.Cluster
// 	// ClusterSpec    *v1alpha1.EKSClusterSpec
// 	Network *cloudv1.Network
// 	// NetworkSpec    *v1alpha1.EKSNetworkSpec
// 	Provider *cloudv1.Provider
// 	// ProviderSpec   *v1alpha1.EKSProviderSpec
// 	// Config         config.RootConfig
// 	ConfigProvider gitops.ConfigProvider
// 	GitOps         gitopsv2.Interface
// 	ConfigManager  gitopssdk.Interface
// 	PDAuth         gitopssdk.PDAuth

// 	// Selector account.Selector

// 	Logger logr.Logger

// 	// BeginAt denotes the starting time of the task.
// 	BeginAt *time.Time
// }
