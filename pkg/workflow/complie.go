package workflow

import (
	"os"
	"regexp"
	"strings"

	"github.com/dop251/goja"
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

func extractVariableNames(jsonStr string) ([]string, error) {
	re := regexp.MustCompile(`{{(.*?)}}`)
	matches := re.FindAllStringSubmatch(jsonStr, -1)
	var variableNames []string
	for _, match := range matches {
		if len(match) > 1 {
			variableNames = append(variableNames, strings.ToLower(match[1]))
		}
	}
	return variableNames, nil
}

func validateDynamicParam(str string) (bool, error) {
	if str == "" {
		return true, nil
	}

	return true, nil
}

func (w *WorkflowFile) Compile(funcs *goja.Runtime) error {
	if w.Vars != nil {
		err := w.Vars.Range(func(_ string, value *Var) error {
			return value.Compile()
		})
		if err != nil {
			return err
		}
	}

	err := w.Brokers.Range(func(_ string, b *Broker) error {
		return b.Compile(w.Vars, w.Clients, w.Channels, &w.Tasks, funcs)
	})
	if err != nil {
		return err
	}

	vars := GetKeyValueVars(w.Vars)
	err = w.Tasks.Range(func(k string, t *Task) error {
		t.Name = k
		err := t.Compile(vars)
		return err
	})
	return err
}

func (w *Workflow) Compile(funcs *goja.Runtime) error {
	if w.Vars != nil {
		err := w.Vars.Range(func(_ string, value *Var) error {
			err := value.Compile()
			return err
		})
		if err != nil {
			return err
		}
	}

	err := w.Brokers.Range(func(_ string, b *Broker) error {
		b.WorkflowID = w.ID
		return b.Compile(w.Vars, w.Clients, w.Channels, &w.Tasks, funcs)
	})
	if err != nil {
		return err
	}
	vars := GetKeyValueVars(w.Vars)
	err = w.Tasks.Range(func(_ string, t *Task) error {
		err := t.Compile(vars)
		return err
	})

	return err
}
