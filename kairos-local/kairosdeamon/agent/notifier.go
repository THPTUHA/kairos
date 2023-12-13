package agent

import (
	"bytes"
	"fmt"
	"net/smtp"

	"github.com/sirupsen/logrus"
)

type notifier struct {
	AgentConfig    *AgentConfig
	Task           *Task
	Execution      *Execution
	ExecutionGroup []*Execution

	logger *logrus.Entry
}

func (n *notifier) report() string {
	var exgStr string
	for _, ex := range n.ExecutionGroup {
		exgStr = fmt.Sprintf("%s\t[Node]: %s [Start]: %s [End]: %s [Success]: %t\n",
			exgStr,
			ex.NodeName,
			ex.StartedAt,
			ex.FinishedAt,
			ex.Success)
	}

	return fmt.Sprintf("Executed: %s\nReporting node: %s\nStart time: %s\nEnd time: %s\nSuccess: %t\nNode: %s\nOutput: %s\nExecution group: %d\n%s",
		n.Execution.TaskID,
		n.AgentConfig.NodeName,
		n.Execution.StartedAt,
		n.Execution.FinishedAt,
		n.Execution.Success,
		n.Execution.NodeName,
		n.Execution.Output,
		n.Execution.Group,
		exgStr)
}

func (n *notifier) statusString() string {
	if n.Execution.Success {
		return "Success"
	}
	return "Failed"
}

func (n *notifier) buildTemplate(templ string) *bytes.Buffer {
	// t, e := template.New("report").Parse(templ)
	// if e != nil {
	// 	n.logger.WithError(e).Error("notifier: error parsing template")
	// 	return bytes.NewBuffer([]byte("Failed to parse template: " + e.Error()))
	// }

	// data := struct {
	// 	Report        string
	// 	TaskID        int64
	// 	ReportingNode string
	// 	StartTime     time.Time
	// 	FinishedAt    time.Time
	// 	Success       string
	// 	NodeName      string
	// 	Output        string
	// }{
	// 	n.report(),
	// 	n.Execution.TaskID,
	// 	n.AgentConfig.NodeName,
	// 	n.Execution.StartedAt,
	// 	n.Execution.FinishedAt,
	// 	fmt.Sprintf("%t", n.Execution.Success),
	// 	n.Execution.NodeName,
	// 	n.Execution.Output,
	// }

	// out := &bytes.Buffer{}
	// err := t.Execute(out, data)
	// if err != nil {
	// 	n.logger.WithError(err).Error("notifier: error executing template")
	// 	return bytes.NewBuffer([]byte("Failed to execute template:" + err.Error()))
	// }
	// return out
	return nil
}

func (n *notifier) auth() smtp.Auth {
	var auth smtp.Auth

	if n.AgentConfig.MailUsername != "" && n.AgentConfig.MailPassword != "" {
		auth = smtp.PlainAuth("", n.AgentConfig.MailUsername, n.AgentConfig.MailPassword, n.AgentConfig.MailHost)
	}

	return auth
}

func (n *notifier) sendExecutionEmail() error {
	// var data *bytes.Buffer
	// if n.AgentConfig.MailPayload != "" {
	// 	data = n.buildTemplate(n.AgentConfig.MailPayload)
	// } else {
	// 	data = bytes.NewBuffer([]byte(n.Execution.Output))
	// }
	// e := &email.Email{
	// 	To:      []string{n.Task.OwnerEmail},
	// 	From:    n.AgentConfig.MailFrom,
	// 	Subject: fmt.Sprintf("%s%s %s execution report", n.AgentConfig.MailSubjectPrefix, n.statusString(), n.Execution.TaskID),
	// 	Text:    []byte(data.Bytes()),
	// 	Headers: textproto.MIMEHeader{},
	// }

	// serverAddr := fmt.Sprintf("%s:%d", n.AgentConfig.MailHost, n.AgentConfig.MailPort)
	// if err := e.Send(serverAddr, n.auth()); err != nil {
	// 	return fmt.Errorf("notifier: Error sending email %s", err)
	// }

	return nil
}

// Send sends the notifications using any configured method
func SendPostNotifications(config *AgentConfig, execution *Execution, exGroup []*Execution, task *Task, logger *logrus.Entry) error {
	// n := &notifier{
	// 	logger: logger,

	// 	AgentConfig:    config,
	// 	Execution:      execution,
	// 	ExecutionGroup: exGroup,
	// 	Task:           task,
	// }

	// var werr error

	// if n.AgentConfig.MailHost != "" && n.AgentConfig.MailPort != 0 && n.Task.OwnerEmail != "" {
	// 	if err := n.sendExecutionEmail(); err != nil {
	// 		werr = multierror.Append(werr, fmt.Errorf("notifier: error sending email: %w", err))
	// 	}
	// }

	// if n.AgentConfig.WebhookEndpoint != "" && n.AgentConfig.WebhookPayload != "" {
	// 	if err := n.callExecutionWebhook(); err != nil {
	// 		werr = multierror.Append(werr, fmt.Errorf("notifier: error posting notification: %w", err))
	// 	}
	// }
	// return werr
	return nil
}
