package plugin

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

// Message types
type nodesMsg []nodeInfo
type podsMsg []podInfo

// UI item types
type item struct {
	title, desc string
}

func (i item) Title() string       { return i.title }
func (i item) Description() string { return i.desc }
func (i item) FilterValue() string { return i.title }

const (
	StateSelectNode    = "selectNode"
	StateSelectAction  = "selectAction"
	StateConfirmCordon = "confirmCordon"
	StateConfirmToggle = "confirmToggle"
	StateConfirm       = "confirm"
	StateSelectPods    = "selectPods"
	StateConfirmPod    = "confirmPod"

	// Actions
	ActionForceDrainNode      = "Force Drain node"
	ActionForceDeleteNonDS    = "Force delete non-daemonset pods"
	ActionForceDeleteSelected = "Force delete selected pods"
	ActionBack                = "Back"

	// Confirmations
	ConfirmYes = "Yes"
	ConfirmNo  = "No"

	// Descriptions
	DescDrainNode           = "Execute drain operation"
	DescForceDeleteNonDS    = "Delete all non-DaemonSet pods"
	DescForceDeleteSelected = "Choose pods to delete"
	DescCancelBack          = "Cancel and go back"
	DescBack                = "Return to previous screen"

	// Messages
	MsgCordon   = "cordon"
	MsgUncordon = "uncordon"
)

type model struct {
	list             list.Model
	spinner          spinner.Model
	width            int
	height           int
	selectedNodeName string
	selectedNode     *corev1.Node
	pods             []podInfo
	selectedPods     map[string]podInfo // key: namespace/name
	state            State
	err              error
	clientset        *kubernetes.Clientset
	quitting         bool
	confirm          bool
	action           string
}

// Constants for key bindings
const (
	KeySpace = " "
	KeyEnter = "enter"
	KeyCtrlC = "ctrl+c"
	KeyQ     = "q"
	KeyEsc   = "esc"
	KeyC     = "c"
)

type nodeInfo struct {
	name        string
	status      string
	schedulable bool
	roles       []string
	age         time.Duration
	version     string
	internal    string
	conditions  []string
}

func (n nodeInfo) Title() string {
	status := "Schedulable"
	if !n.schedulable {
		status = "Cordoned"
	}
	return fmt.Sprintf("%s (%s)", n.name, status)
}

func (n nodeInfo) Description() string {
	return fmt.Sprintf("Status: %s | Roles: %s | Age: %s | Version: %s | InternalIP: %s | Conditions: %s",
		n.status,
		strings.Join(n.roles, ","),
		formatDuration(n.age),
		n.version,
		n.internal,
		strings.Join(n.conditions, ","),
	)
}

func (n nodeInfo) FilterValue() string {
	return n.name
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Minute)
	h := d / time.Hour
	d -= h * time.Hour
	m := d / time.Minute

	if h > 0 {
		if h >= 24 {
			days := h / 24
			return fmt.Sprintf("%dd", days)
		}
		return fmt.Sprintf("%dh", h)
	}
	return fmt.Sprintf("%dm", m)
}

type podInfo struct {
	name      string
	namespace string
	owner     string
	ownerKind string
	phase     string
	age       time.Duration
	selected  bool
}

func (p podInfo) Title() string {
	prefix := "[ ]"
	if p.selected {
		prefix = "[✓]"
	}
	return fmt.Sprintf("%s %s", prefix, p.name)
}

func (p podInfo) Description() string {
	owner := p.owner
	if owner == "" {
		owner = "<none>"
	}
	return fmt.Sprintf("Namespace: %s | Phase: %s | Owner: %s(%s) | Age: %s",
		p.namespace,
		p.phase,
		owner,
		p.ownerKind,
		formatDuration(p.age),
	)
}

func (p podInfo) FilterValue() string {
	return p.namespace + "/" + p.name
}

type State string
