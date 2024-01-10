package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/smtp"
	"strconv"
	"time"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
)

type Notification struct{}

func (n *Notification) Execute(args *proto.ExecuteRequest, cb plugin.StatusHelper) (*proto.ExecuteResponse, error) {

	out, err := n.ExecuteImpl(args)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, err
}

func formatTimestampToDateString(timestamp int64) string {
	t := time.Unix(timestamp, 0)
	formattedDate := t.Format("15:04:05 02-01-2006")

	return formattedDate
}

func (n *Notification) ExecuteImpl(args *proto.ExecuteRequest) ([]byte, error) {
	from := "badao04122001@gmail.com"
	password := "utuu udbi pyov qhaj"
	smtpHost := "smtp.gmail.com"
	smtpPort := "587"
	fmt.Printf("config %+v\n", args.Config)
	receivers := args.Config["receivers"]
	success := args.Config["success"]
	output := args.Config["output"]

	finishedAt := helper.GetTimeNow()

	suc, err := strconv.ParseBool(success)
	if err != nil {
		return nil, err
	}

	to := make([]string, 0)

	if receivers == "" {
		return nil, fmt.Errorf("Empty receivers payload")
	}

	err = json.Unmarshal([]byte(receivers), &to)
	if err != nil {
		return nil, err
	}

	if len(to) == 0 {
		return nil, fmt.Errorf("Empty receiver")
	}

	subject := "Deploy service"

	var body bytes.Buffer
	var content bytes.Buffer
	mimeHeaders := "MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n"

	status := `<span style="color: green;">Success</span>`
	if !suc {
		status = `<span style="color: red;">Fault</span>`
	}
	temp := fmt.Sprintf(`<html>
							<body>
								<div>Finish at {{.FinishedAt}}</div>
								<div>%s {{.Message}}</div>
							</body>
						</html>`, status)

	body.Write([]byte(fmt.Sprintf("Subject: %s \n%s\n\n %s\n\n", subject, mimeHeaders, temp)))
	t, err := template.New("report").Parse(body.String())
	t.Execute(&content, struct {
		FinishedAt string
		Message    string
		Status     string
	}{
		FinishedAt: formatTimestampToDateString(finishedAt),
		Message:    output,
		Status:     status,
	})
	fmt.Println(content.String())

	auth := smtp.PlainAuth("", from, password, smtpHost)

	err = smtp.SendMail(smtpHost+":"+smtpPort, auth, from, to, content.Bytes())
	if err != nil {
		return nil, err
	}
	return []byte("send mail successful"), nil
}
