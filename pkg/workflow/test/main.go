package main

import (
	"fmt"

	"github.com/dop251/goja"
)

func main() {
	// s := `
	// {{define .fuck map | put ("hello") (1) | put ("hello2") (2) | .fuck}}
	// 	{{define .arrs array | push (5) | push (3) | push (4) | .arrs }}
	// 		{{define .result number}}
	// 		{{range .fuck .f_key .f_value}}
	// 			{{range .arrs .a_key .a_value}}
	// 				{{merge .result .f_value | merge .a_value | .result}}{{end}}
	// 			{{end}}
	// 		{{end}}
	// 	{{printf ("result  %v") .result}}

	// 	{{end}}
	// {{end}}
	// `

	// s := `
	// {{define .fuck map | put ("hello") (1) | put ("hello2") (2) | .fuck}}
	// 	{{define .arrs array | push (5) | push (3) | push (4) | .arrs }}
	// 		{{define .result number}}
	// 		{{range .fuck .f_key .f_value}}
	// 			{{range .arrs .a_key .a_value}}
	// 				{{printf ("fuck key %v number \narrs key %v") .f_key .a_key}}
	// 			{{end}}
	// 		{{end}}
	// 	{{end}}
	// {{end}}
	// `

	// s := `
	// {{define .mp array | push ("abc") (10)}}
	// {{end}}
	// {{define .a string | set .a ("OK")}}

	// {{put .mp (12) }}
	// 	{{merge .a (100)}}{{end}}
	// 	{{printf ("run here")}}
	// {{else}}
	// 	{{define .a number | set .a (10)}}
	// 	{{merge .a (11) | .a}}{{end}}
	// 	{{printf ("no")}}
	// {{end}}
	// {{printf ("a=%v") .a}}
	// `

	// input := []string{
	// 	"a_channel", "b_channel",
	// }

	vm := goja.New()

	script := `
		function sum(a,b) {

			return a+b;
		}
	`

	prog, err := goja.Compile("", script, true)
	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}
	_, err = vm.RunProgram(prog)

	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}

	script2 := `
		function multi(a,b) {

			return a*b;
		}
	`
	prog2, err := goja.Compile("", script2, true)
	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}
	_, err = vm.RunProgram(prog2)
	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}

	var multi goja.Callable
	err = vm.ExportTo(vm.Get("multi"), &multi)
	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}

	var sum goja.Callable
	err = vm.ExportTo(vm.Get("sum"), &sum)
	if err != nil {
		fmt.Printf("Error compiling the script %v ", err)
		return
	}

	items := make([]goja.Value, 0)
	items = append(items, vm.ToValue(2), vm.ToValue(10))
	result1, err := sum(goja.Undefined(), items...)
	result2, err := multi(goja.Undefined(), items...)
	fmt.Println(result1, result2)
	// t := workflow.NewTemplate(vm)
	// var vars workflow.Vars
	// vars.Set("c", &workflow.Var{Value: "1"})
	// vars.Set("zero", &workflow.Var{Value: "0"})
	// vars.Set("item_value", &workflow.Var{Value: "0"})
	// vars.Set("item_index", &workflow.Var{Value: "0"})

	// err = t.Build(input, s, &vars, true)
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// run
	// var output workflow.Vars
	// output.Set(".REQUEST_POST_CHANNEL_MSG", &workflow.Var{
	// 	Value: "hello",
	// })
	// output.Set(".REQUEST_POST_CHANNEL_CMD", &workflow.Var{
	// 	Value: "hello 2",
	// })
	// output.Set(".CRAWL_DATA_TASK_STATUS", &workflow.Var{
	// 	Value: "2",
	// })
	// options := make(map[string]workflow.ReplyData)
	// at := workflow.ReplyData{
	// 	"content": map[string]interface{}{
	// 		"msg": map[string]interface{}{
	// 			"user_id": 1,
	// 		},
	// 	},
	// }

	// options["a_channel"] = at

	// r := t.NewRutime(options)

	// r.Execute()
	// fmt.Println("deliver", d)
	// fmt.Println("err", e)

}
