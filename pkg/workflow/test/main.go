package main

import (
	"fmt"

	"github.com/THPTUHA/kairos/pkg/workflow"
)

func main() {
	s := `
	{{set .ABCD .REQUEST_POST_CHANNEL_MSG}}
	{{if eq .REQUEST_POST_CHANNEL_MSG .REQUEST_POST_CHANNEL_CMD}}
		{{set .ABC .REQUEST_POST_CHANNEL_MSG}}
		{{sum .ABCD | sub .REQUEST_POST_CHANNEL | .RESULT }}
	    {{send .REQUEST_POST_CHANNEL .CRAWL_DATA_TASK}}
	{{elseif eq .CRAWL_DATA_TASK_STATUS .TASK_STATUS_SUCCESS }}
		{{range .REQUEST_POST_CHANNEL_MSG}}
			{{if eq .REQUEST_POST_CHANNEL_MSG .REQUEST_POST_CHANNEL_CMD}}
				{{set .ABC .REQUEST_POST_CHANNEL_MSG}}
				{{sum .ABCD | sub .REQUEST_POST_CHANNEL | .RESULT }}
				{{send .REQUEST_POST_CHANNEL .CRAWL_DATA_TASK}}
			{{elseif eq .CRAWL_DATA_TASK_STATUS .TASK_STATUS_SUCCESS }}
				{{range .REQUEST_POST_CHANNEL_MSG}}
				
				{{endrange}}
			{{elseif eq .CRAWL_DATA_TASK_STATUS .TASK_STATUS_SUCCESS}}
			{{endif}}
		{{endrange}}
	{{elseif eq .CRAWL_DATA_TASK_STATUS .TASK_STATUS_SUCCESS }}
	{{endif}}
	`
	input := []string{
		"request_post_channel", "crawl_data_task",
	}
	t := workflow.NewTemplate()
	var vars workflow.Vars
	err := t.Build(input, s, &vars)
	if err != nil {
		fmt.Println(err)
		return
	}
	// run
	var output workflow.Vars
	output.Set(".REQUEST_POST_CHANNEL_MSG", &workflow.Var{
		Value: "hello",
	})
	output.Set(".REQUEST_POST_CHANNEL_CMD", &workflow.Var{
		Value: "hello 2",
	})
	output.Set(".CRAWL_DATA_TASK_STATUS", &workflow.Var{
		Value: "2",
	})
	// d, e := t.Execute(&output)
	// fmt.Println("deliver", d)
	// fmt.Println("err", e)

}
