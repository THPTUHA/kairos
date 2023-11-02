package deliverer

type emulationSurveyHandler struct {
	node *Node
}

func newEmulationSurveyHandler(node *Node) *emulationSurveyHandler {
	return &emulationSurveyHandler{node: node}
}
