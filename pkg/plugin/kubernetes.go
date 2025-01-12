package plugin

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
)

// DrainerOption defines function type for configuring drain.Helper
type DrainerOption func(*drain.Helper)

// WithForce sets the Force option
func WithForce(force bool) DrainerOption {
	return func(h *drain.Helper) {
		h.Force = force
	}
}

// WithGracePeriod sets the GracePeriodSeconds
func WithGracePeriod(seconds int) DrainerOption {
	return func(h *drain.Helper) {
		h.GracePeriodSeconds = seconds
	}
}

// WithTimeout sets the Timeout duration
func WithTimeout(timeout time.Duration) DrainerOption {
	return func(h *drain.Helper) {
		h.Timeout = timeout
	}
}

// WithDeleteEmptyDir sets the DeleteEmptyDirData option
func WithDeleteEmptyDir(delete bool) DrainerOption {
	return func(h *drain.Helper) {
		h.DeleteEmptyDirData = delete
	}
}

// WithIgnoreDaemonSets sets the IgnoreAllDaemonSets option
func WithIgnoreDaemonSets(ignore bool) DrainerOption {
	return func(h *drain.Helper) {
		h.IgnoreAllDaemonSets = ignore
	}
}

// getNodes retrieves the list of nodes from the cluster
// Returns a tea.Cmd that will fetch the nodes asynchronously
func getNodes(clientset *kubernetes.Clientset) tea.Cmd {
	return func() tea.Msg {
		if clientset == nil {
			return fmt.Errorf("kubernetes client is not initialized")
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		nodeList, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}

		if len(nodeList.Items) == 0 {
			return fmt.Errorf("no nodes found in the cluster")
		}

		var nodes []nodeInfo
		for _, node := range nodeList.Items {
			ready := "NotReady"
			var conditions []string
			for _, condition := range node.Status.Conditions {
				if condition.Status == corev1.ConditionTrue {
					conditions = append(conditions, string(condition.Type))
				}
				if condition.Type == corev1.NodeReady {
					if condition.Status == corev1.ConditionTrue {
						ready = "Ready"
					}
					break
				}
			}

			// Get node roles
			roles := getNodeRoles(node.Labels)
			if len(roles) == 0 {
				roles = []string{"<none>"}
			}

			// Get age
			age := time.Since(node.CreationTimestamp.Time).Round(time.Second)

			nodes = append(nodes, nodeInfo{
				name:        node.Name,
				status:      ready,
				schedulable: !node.Spec.Unschedulable,
				roles:       roles,
				age:         age,
				version:     node.Status.NodeInfo.KubeletVersion,
				internal:    node.Status.Addresses[0].Address,
				conditions:  conditions,
			})
		}
		return nodesMsg(nodes)
	}
}

func getNodeRoles(labels map[string]string) []string {
	var roles []string
	for label := range labels {
		if strings.HasPrefix(label, "node-role.kubernetes.io/") {
			role := strings.TrimPrefix(label, "node-role.kubernetes.io/")
			roles = append(roles, role)
		}
	}
	sort.Strings(roles)
	return roles
}

func getPods(clientset *kubernetes.Clientset, nodeName string) tea.Cmd {
	return func() tea.Msg {
		podList, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
			FieldSelector: fmt.Sprintf("spec.nodeName=%s", nodeName),
		})
		if err != nil {
			return err
		}
		var pods []podInfo
		for _, pod := range podList.Items {
			var owner, ownerKind string
			if len(pod.OwnerReferences) > 0 {
				owner = pod.OwnerReferences[0].Name
				ownerKind = pod.OwnerReferences[0].Kind
			}

			pods = append(pods, podInfo{
				name:      pod.Name,
				namespace: pod.Namespace,
				owner:     owner,
				ownerKind: ownerKind,
				phase:     string(pod.Status.Phase),
				age:       time.Since(pod.CreationTimestamp.Time),
			})
		}
		return podsMsg(pods)
	}
}

// newDrainer creates a new drain.Helper with default settings and applies given options
func newDrainer(clientset *kubernetes.Clientset, opts ...DrainerOption) *drain.Helper {
	drainer := &drain.Helper{
		Ctx:                 context.TODO(),
		Client:              clientset,
		Force:               true,
		GracePeriodSeconds:  -1, // Use pod's grace period
		IgnoreAllDaemonSets: true,
		Timeout:             30 * time.Second,
		DeleteEmptyDirData:  true,
		Out:                 os.Stdout,
		ErrOut:              os.Stderr,
	}

	// Apply all provided options
	for _, opt := range opts {
		opt(drainer)
	}

	return drainer
}

func getNode(clientset *kubernetes.Clientset, nodeName string) (*corev1.Node, error) {
	node, err := clientset.CoreV1().Nodes().Get(context.TODO(), nodeName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return node, nil
}

func deletePod(clientset *kubernetes.Clientset, namespace, name string) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := clientset.CoreV1().Pods(namespace).Delete(ctx, name, metav1.DeleteOptions{
			GracePeriodSeconds: new(int64),
		})
		if err != nil {
			return fmt.Errorf("failed to delete pod %s/%s: %v", namespace, name, err)
		}
		fmt.Printf("Successfully deleted pod %s/%s\n", namespace, name)
		return nil
	}
}

func isDaemonSetPod(pod corev1.Pod) bool {
	for _, owner := range pod.OwnerReferences {
		if owner.Kind == "DaemonSet" {
			return true
		}
	}
	return false
}
