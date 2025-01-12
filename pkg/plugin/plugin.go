package plugin

import (
	tea "github.com/charmbracelet/bubbletea"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Plugin struct {
	clientset *kubernetes.Clientset
}

func NewPlugin(config *rest.Config) (*Plugin, error) {
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}
	return &Plugin{
		clientset: clientset,
	}, nil
}

func (p *Plugin) Run() error {
	program := tea.NewProgram(
		initialModel(p.clientset),
		tea.WithAltScreen(),
		tea.WithMouseCellMotion(),
	)
	_, err := program.Run()
	return err
}
