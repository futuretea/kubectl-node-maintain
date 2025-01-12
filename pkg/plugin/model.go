package plugin

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/kubectl/pkg/drain"
)

func initialModel(clientset *kubernetes.Clientset) model {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Get initial terminal size
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		// Default to 80x24
		w, h = 80, 24
	}

	l := createList([]list.Item{}, "Loading nodes...", w, h)

	return model{
		spinner:      s,
		list:         l,
		width:        w,
		height:       h,
		state:        StateSelectNode,
		clientset:    clientset,
		confirm:      false,
		selectedPods: make(map[string]podInfo),
	}
}

func (m model) Init() tea.Cmd {
	return tea.Sequence(
		tea.EnterAltScreen,
		tea.Batch(m.spinner.Tick, getNodes(m.clientset)),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			m.quitting = true
			return m, tea.Sequence(tea.ExitAltScreen, tea.Quit)
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		if m.list.Items() == nil {
			return m, nil
		}
		h, v := lipgloss.NewStyle().Margin(1, 2).GetFrameSize()
		m.list.SetSize(m.width-h, m.height-v)

	case error:
		m.err = msg
		return m, nil

	case nodesMsg:
		items := make([]list.Item, 0, len(msg))
		for _, node := range msg {
			items = append(items, node)
		}

		m.list = createList(items, "Select Node", m.width, m.height)
		return m, nil

	case podsMsg:
		m.pods = msg
		items := make([]list.Item, len(msg))
		for i := range msg {
			items[i] = msg[i]
		}

		m.list = createList(items, "Select Pods", m.width, m.height)
		return m, nil
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	switch m.state {
	case StateSelectNode:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case KeyC:
				if m.list.SelectedItem() != nil {
					node := m.list.SelectedItem().(nodeInfo)
					m.selectedNodeName = node.name
					m.selectedNode, _ = getNode(m.clientset, node.name)
					m.state = StateConfirmToggle
					action := MsgCordon
					if !node.schedulable {
						action = MsgUncordon
					}
					items := []list.Item{
						item{title: ConfirmYes, desc: fmt.Sprintf("Confirm %s node %s", action, node.name)},
						item{title: ConfirmNo, desc: DescCancelBack},
					}
					m.list = createList(items, fmt.Sprintf("Confirm %s Operation", action), m.width, m.height)
					return m, nil
				}
			case KeyEnter:
				if m.list.SelectedItem() != nil {
					m.selectedNodeName = m.list.SelectedItem().(nodeInfo).name
					m.selectedNode, _ = getNode(m.clientset, m.selectedNodeName)
					m.state = StateSelectAction
					items := []list.Item{
						item{title: ActionForceDrainNode, desc: DescDrainNode},
						item{title: ActionForceDeleteNonDS, desc: DescForceDeleteNonDS},
						item{title: ActionForceDeleteSelected, desc: DescForceDeleteSelected},
						item{title: ActionBack, desc: DescBack},
					}
					m.list = createList(items, "Select Operation", m.width, m.height)
				}
			}
		}

	case StateSelectAction:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				if m.list.SelectedItem() != nil {
					m.action = m.list.SelectedItem().(item).Title()

					if m.action == ActionBack {
						m.state = StateSelectNode
						return m, getNodes(m.clientset)
					}

					// First confirm cordon operation
					m.state = StateConfirmCordon
					items := []list.Item{
						item{title: ConfirmYes, desc: fmt.Sprintf("Confirm cordon node %s before %s", m.selectedNodeName, m.action)},
						item{title: ConfirmNo, desc: DescCancelBack},
					}
					m.list = createList(items, "Confirm Cordon Operation", m.width, m.height)
					return m, nil
				}
			}
		}

	case StateConfirmCordon:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				if m.list.SelectedItem() != nil {
					confirm := m.list.SelectedItem().(item).Title()
					if confirm == ConfirmYes {
						drainer := newDrainer(m.clientset)
						if err := drain.RunCordonOrUncordon(drainer, m.selectedNode, true); err != nil {
							m.err = err
							return m, nil
						}
						fmt.Printf("Successfully cordoned node %s\n", m.selectedNode.Name)

						// After cordon, proceed to confirm the main operation
						if m.action == ActionForceDeleteSelected {
							m.state = StateSelectPods
							return m, getPods(m.clientset, m.selectedNodeName)
						}

						m.state = StateConfirm
						items := []list.Item{
							item{title: ConfirmYes, desc: fmt.Sprintf("Confirm %s on node %s", m.action, m.selectedNodeName)},
							item{title: ConfirmNo, desc: DescCancelBack},
						}
						m.list = createList(items, "Confirm Operation", m.width, m.height)
					} else {
						// Go back to action selection
						m.state = StateSelectAction
						items := []list.Item{
							item{title: ActionForceDrainNode, desc: DescDrainNode},
							item{title: ActionForceDeleteNonDS, desc: DescForceDeleteNonDS},
							item{title: ActionForceDeleteSelected, desc: DescForceDeleteSelected},
							item{title: ActionBack, desc: DescBack},
						}
						m.list = createList(items, "Select Operation", m.width, m.height)
					}
					return m, nil
				}
			}
		} else if keyMsg.String() == KeyEsc {
			m.state = StateSelectAction
			items := []list.Item{
				item{title: ActionForceDrainNode, desc: DescDrainNode},
				item{title: ActionForceDeleteNonDS, desc: DescForceDeleteNonDS},
				item{title: ActionForceDeleteSelected, desc: DescForceDeleteSelected},
				item{title: ActionBack, desc: DescBack},
			}
			m.list = createList(items, "Select Operation", m.width, m.height)
			return m, nil
		}

	case StateConfirm:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				if m.list.SelectedItem() != nil {
					confirm := m.list.SelectedItem().(item).Title()
					if confirm == ConfirmYes {
						drainer := newDrainer(m.clientset)
						if err := drain.RunCordonOrUncordon(drainer, m.selectedNode, true); err != nil {
							m.err = err
							return m, nil
						}
						fmt.Printf("Successfully cordoned node %s\n", m.selectedNode.Name)

						switch m.action {
						case ActionForceDrainNode:
							return m, func() tea.Msg {
								if err := drain.RunNodeDrain(drainer, m.selectedNode.Name); err != nil {
									return fmt.Errorf("failed to drain node %s: %v", m.selectedNodeName, err)
								}
								fmt.Printf("Successfully drained node %s\n", m.selectedNodeName)
								m.quitting = true
								return nil
							}
						case ActionForceDeleteNonDS:
							return m, func() tea.Msg {
								pods, err := m.clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
									FieldSelector: fmt.Sprintf("spec.nodeName=%s", m.selectedNodeName),
								})
								if err != nil {
									return fmt.Errorf("failed to get pods on node %s: %v", m.selectedNodeName, err)
								}

								for _, pod := range pods.Items {
									if !isDaemonSetPod(pod) {
										err = m.clientset.CoreV1().Pods(pod.Namespace).Delete(context.TODO(), pod.Name, metav1.DeleteOptions{
											GracePeriodSeconds: new(int64),
										})
										if err != nil {
											fmt.Printf("Failed to delete pod %s/%s: %v\n", pod.Namespace, pod.Name, err)
										} else {
											fmt.Printf("Successfully deleted pod %s/%s\n", pod.Namespace, pod.Name)
										}
									}
								}
								return nil
							}
						}
					} else {
						// Go back to action selection
						m.state = StateSelectAction
						items := []list.Item{
							item{title: ActionForceDrainNode, desc: DescDrainNode},
							item{title: ActionForceDeleteNonDS, desc: DescForceDeleteNonDS},
							item{title: ActionForceDeleteSelected, desc: DescForceDeleteSelected},
							item{title: ActionBack, desc: DescBack},
						}
						m.list = createList(items, "Select Operation", m.width, m.height)
					}
				}
			}
		}

	case StateSelectPods:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			switch keyMsg.String() {
			case KeySpace:
				if m.list.SelectedItem() != nil {
					pod := m.list.SelectedItem().(podInfo)
					key := pod.namespace + "/" + pod.name
					// Update pod selection status in the pods list
					for i := range m.pods {
						if m.pods[i].name == pod.name && m.pods[i].namespace == pod.namespace {
							m.pods[i].selected = !m.pods[i].selected
							break
						}
					}
					// Update the list items to reflect the selection
					currentIndex := m.list.Index()
					items := make([]list.Item, len(m.pods))
					for i := range m.pods {
						items[i] = m.pods[i]
					}
					m.list.SetItems(items)
					m.list.Select(currentIndex)

					if _, exists := m.selectedPods[key]; exists {
						delete(m.selectedPods, key)
					} else {
						m.selectedPods[key] = pod
					}
					return m, nil
				}
			case KeyEnter:
				if len(m.selectedPods) > 0 {
					// Show confirmation for pod deletion
					m.state = StateConfirmPod
					items := []list.Item{
						item{title: ConfirmYes, desc: fmt.Sprintf("Confirm delete %d selected pods", len(m.selectedPods))},
						item{title: ConfirmNo, desc: DescCancelBack},
					}
					m.list = createList(items, "Confirm Pod Deletion", m.width, m.height)
				}
			}
		}

	case StateConfirmPod:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == "enter" {
				if m.list.SelectedItem() != nil {
					confirm := m.list.SelectedItem().(item).Title()
					if confirm == ConfirmYes {
						drainer := newDrainer(m.clientset)
						if err := drain.RunCordonOrUncordon(drainer, m.selectedNode, true); err != nil {
							m.err = err
							return m, nil
						}
						fmt.Printf("Successfully cordoned node %s\n", m.selectedNode.Name)
						// Delete all selected pods
						var cmds []tea.Cmd
						for _, pod := range m.selectedPods {
							cmds = append(cmds, deletePod(m.clientset, pod.namespace, pod.name))
						}
						cmds = append(cmds, func() tea.Msg {
							m.quitting = true
							return nil
						})
						return m, tea.Sequence(cmds...)
					} else {
						// Go back to action selection
						m.state = StateSelectAction
						items := []list.Item{
							item{title: ActionForceDrainNode, desc: DescDrainNode},
							item{title: ActionForceDeleteNonDS, desc: DescForceDeleteNonDS},
							item{title: ActionForceDeleteSelected, desc: DescForceDeleteSelected},
							item{title: ActionBack, desc: DescBack},
						}
						m.list = createList(items, "Select Operation", m.width, m.height)
					}
				}
			}
		}

	case StateConfirmToggle:
		if keyMsg, ok := msg.(tea.KeyMsg); ok {
			if keyMsg.String() == KeyEnter {
				if m.list.SelectedItem() != nil {
					confirm := m.list.SelectedItem().(item).Title()
					if confirm == ConfirmYes {
						node := m.selectedNode
						drainer := newDrainer(m.clientset)
						// Toggle cordon state
						if err := drain.RunCordonOrUncordon(drainer, node, !node.Spec.Unschedulable); err != nil {
							m.err = err
							return m, nil
						}
						action := MsgCordon
						if node.Spec.Unschedulable {
							action = MsgUncordon
						}
						fmt.Printf("Successfully %sed node %s\n", action, node.Name)
					}
					// Return to node selection with refreshed list
					m.state = StateSelectNode
					return m, getNodes(m.clientset)
				}
			} else if keyMsg.String() == KeyEsc {
				m.state = StateSelectNode
				return m, getNodes(m.clientset)
			}
		}
	}

	return m, cmd
}

func (m model) View() string {
	helpStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	var help string
	if m.state == StateSelectPods {
		help = helpStyle.Render("↑/↓: Navigate • space: Toggle select • enter: Confirm • /: Filter • q: Quit")
	} else if m.state == StateSelectNode {
		help = helpStyle.Render("↑/↓: Navigate • c: Toggle cordon • enter: Select • /: Filter • q: Quit")
	} else {
		help = helpStyle.Render("↑/↓: Navigate • enter: Select • esc: Back • /: Filter • q: Quit")
	}

	if m.err != nil {
		return fmt.Sprintf("\nError: %v\nPress 'q' or Ctrl+C to exit\n", m.err)
	}

	if m.quitting {
		return "Operation completed. Goodbye!\n"
	}

	var status string
	switch m.state {
	case StateSelectNode:
		if len(m.list.Items()) == 0 {
			status = m.spinner.View() + " Loading nodes..."
		}
	case "selectPods":
		if len(m.list.Items()) == 0 {
			status = m.spinner.View() + " Loading pods..."
		}
	}

	if status != "" {
		return "\n" + status + "\n"
	}

	return "\n" + m.list.View() + "\n" + help
}
