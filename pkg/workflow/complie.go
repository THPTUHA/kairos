package workflow

import (
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

func ComplieFile(file string) (*WorkflowFile, error) {
	var wf WorkflowFile
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(b, &wf)
	if err != nil {
		return nil, err
	}

	return &wf, nil
}

func ComplieByte(b []byte) (*WorkflowFile, error) {
	var wf WorkflowFile
	err := yaml.Unmarshal(b, &wf)
	if err != nil {
		return nil, err
	}

	return &wf, nil
}

func extractVariables(templateString string) []string {
	re := regexp.MustCompile(`{{\s*\.([a-zA-Z0-9_]+)\s*}}`)

	matches := re.FindAllStringSubmatch(templateString, -1)

	var variables []string
	for _, match := range matches {
		variables = append(variables, match[1])
	}

	return variables
}
