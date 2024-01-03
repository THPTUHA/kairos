import { useEffect, useRef, useState } from "react"
import { IoIosAdd, IoMdAddCircleOutline } from "react-icons/io"
import { useAsync } from "react-use"
import { services } from "../services"
import { Input, Popconfirm, Radio, Table, Tooltip } from "antd"
import { ColumnGroupType, ColumnsType } from "antd/es/table"
import { formatDate } from "../helper/date"
import { Toast } from "../components/Toast"
import Modal from "react-responsive-modal"
import { Function } from "../models/function"
import { UploadButton } from "../components/UploadButton"
import CodeEditor from '@uiw/react-textarea-code-editor';
import { MdDelete } from "react-icons/md"
import { BiSolidShow } from "react-icons/bi"
import { VscDebugAltSmall } from "react-icons/vsc";
import { parseCode } from "../helper/base"
import { colorElement } from "../helper/element"
import ReactJson from "react-json-view"
import { FaArrowRight, FaDeleteLeft, FaPlay } from "react-icons/fa6"

const locale = {
    emptyText: <span>Empty function</span>,
};

function checkAndGetFunction(code: string) {
    const functionRegex = /function\s*([a-zA-Z_$][0-9a-zA-Z_$]*)\s*\([^)]*\)\s*\{[^]*\}/;
    const matches = code.match(functionRegex);
    if (matches) {
        if (matches.length > 2) {
            return Error("only one function")
        }
        return matches[1];
    } else {
        return Error("can't find function");
    }
}


const FuntionPage = () => {
    const [clientSelected, setClientSelected] = useState(0)
    const [error, setError] = useState<Error>();
    const [showModalForm, setShowModalForm] = useState(false)
    const [showDebug, setShowDebug] = useState(false)
    const [reload, setReload] = useState(0)
    const [view, setView] = useState(false)
    const [code, setCode] = useState("")

    const fuctions = useAsync(async () => {
        const ws = await services.functions
            .list()
            .catch(setError)
        if (Array.isArray(ws)) {
            return ws
        }
        return []
    }, [reload])

    const rowSelection = {
        onChange: (_: React.Key[], selectedRows: unknown[]) => {
            setClientSelected(selectedRows.length)
        },
    };

    useEffect(() => {
        if (error) {
            console.log("ERROR", error)
            Toast.error(error.message)
        }
    }, [error])

    const columns: ColumnsType<Function> = [
        {
            title: 'Name',
            dataIndex: 'name',
        },
        {
            title: 'Created at',
            dataIndex: 'created_at',
            render: (value: number) => {
                return <>{formatDate(value)}</>
            }
        },
        {
            title: 'Action',
            dataIndex: 'created_at',
            render: (_: any, record: Function) => {
                return (
                    <div className="flex">
                        <Popconfirm
                            title={"Destroy function"}
                            onConfirm={() => {
                                services.functions.delete(record.id).then().catch()
                                setReload(reload + 1)
                            }}
                            okText={"yes"}
                            cancelText={"no"}
                        >
                            <Tooltip title={"delete"}>
                                <MdDelete className="w-6 h-6 cursor-pointer" />
                            </Tooltip>
                        </Popconfirm>
                        <Tooltip title={"detail"}>
                            <BiSolidShow className="w-6 h-6 cursor-pointer"
                                onClick={() => {
                                    setView(true)
                                    setCode(record.content)
                                    setShowModalForm(true)
                                }}
                            />
                        </Tooltip>

                    </div>
                )
            }
        },
    ];


    function onCreateFuntion(e: any) {
        console.log(e)
        if (e && e.err) {
            Toast.error(e.err)
            return
        }
        setReload(e => e + 1)
        setView(false)
        setShowModalForm(false)
    }

    function createFunction() {
        console.log({ code })
        if (code) {
            const valid = checkAndGetFunction(code)
            console.log(valid)

            if (typeof valid == "object") {
                Toast.error(valid.message)
                return
            }
            services.functions
                .create({
                    name: valid,
                    content: code
                })
                .then(onCreateFuntion)
                .catch(setError);
        }
    }

    return (
        <div>
            <div className="flex">
                <span className="flex items-center bg-blue-500 w-40 justify-center rounded py-1 cursor-pointer"
                    onClick={() => {
                        setCode("")
                        setShowModalForm(true)
                    }}
                >
                    <IoIosAdd className="w-6 h-6" />
                    <span>Create function</span>
                </span>
                <span className="flex mx-2 items-center bg-blue-500 w-32 justify-center rounded py-1 cursor-pointer"
                    onClick={() => {
                        setShowDebug(true)
                    }}
                >
                    <VscDebugAltSmall className="w-6 h-6" />
                    <span>Debug</span>
                </span>
                {clientSelected > 0 ?
                    <span className="flex items-center">
                        <button className="bg-red-500 w-12 rounded ml-2 py-1">Delete</button>
                        <span>{clientSelected} selected</span>
                    </span>
                    : ""}
            </div>

            {
                fuctions.value ?
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={fuctions.value}
                        locale={locale}
                    /> : <div>Loading</div>
            }

            <Modal
                open={showModalForm}
                onClose={() => { setShowModalForm(false) }}
                center
            >
                <div className="min-w-[800px]">
                    <CodeEditor
                        value={code}
                        minHeight={300}
                        language="js"
                        placeholder="Please enter JS code."
                        onChange={(evn) => {
                            setCode(evn.target.value)
                        }}
                        padding={15}
                        style={{
                            backgroundColor: "#f5f5f5",
                            fontFamily: 'ui-monospace,SFMono-Regular,SF Mono,Consolas,Liberation Mono,Menlo,monospace',
                        }}
                    />
                    <div className="flex items-center">
                        <button onClick={createFunction}
                            className="font-bold mr-6"
                        > Apply</button>
                        <UploadButton onUpload={setCode} onError={setError} />
                    </div>
                </div>

            </Modal>
            <Modal
                open={showDebug}
                onClose={() => { setShowDebug(false) }}
                center
            >
                <DebugTool />
            </Modal>
        </div>
    )
}

const DebugTool = () => {
    const [code, setCode] = useState("")
    const [input, setInput] = useState<any>([])
    const [output, setOuput] = useState("")
    const [err, setError] = useState("")
    const [editor, setEditor] = useState<any[]>([])
    const editorRef = useRef<HTMLDivElement | null>(null);
    const [openInput, setOpenInput] = useState(false)
    const [inName, setInName] = useState("")
    const [text, setText] = useState('');
    const [result, setResult] = useState<any>({});
    const [inputType, setInputType] = useState(1)

    const handleInputChange = (e: React.ChangeEvent<HTMLTextAreaElement>) => {
        setText(e.target.value);
        autoExpand(e.target); //
        setEditor(parseCode(e.target.value))
    };

    const autoExpand = (element: HTMLTextAreaElement) => {
        element.style.height = 'auto'; 
        element.style.height = `${element.scrollHeight}px`; 
    };

    const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
       
        if (e.key === 'Tab') {
            e.preventDefault();
            //@ts-ignore
            const { selectionStart, selectionEnd } = e.target;
            const newText =
              text.substring(0, selectionStart) + '    ' + text.substring(selectionEnd);
      
            setText(newText);
             //@ts-ignore
            e.target.setSelectionRange(selectionStart + 4, selectionStart + 4);
          }
    };

    const handleRun = async () => {
        console.log({
            flows: text,
            input: input
        })
        const temp :any= {}
        for(const item of input){
            temp[item.name] = item.value
        }
        setResult({running: true})
        const result = await services.functions.debug({
            flows: text,
            input: temp
        })
            .catch(setError)
        if (result) {
            if(result.tracking){
                result.tracking = JSON.parse(result.tracking)
            }
            console.log("RESULT", result)
            setResult(result)
        }
    }

    return (
        <div className="min-w-[800px] ">
            <div className="border-2 border-blue-500 border-dashed px-2 py-2 w-11/12">
                <div className="flex items-center ">
                    <div>Input</div>
                    <button className="mx-2" onClick={() => setOpenInput(true)}><IoMdAddCircleOutline /></button>
                </div>
                {
                    input.map((item: any, idx: number) => (
                        <div key={item.name} className="flex items-center">
                            <div>{item.name}</div>
                            <ReactJson
                                src={item.value}
                                name={false}
                                enableClipboard={false}
                                onAdd={(e) => {
                                    setInput((input:any)=>{
                                        if (e.updated_src) {
                                            const newInput = [...input]
                                            newInput[idx].value = e.updated_src
                                            console.log("NEW INPU==", newInput)
                                            return newInput
                                        }
                                        return input
                                    })
                                }}
                                onDelete={(e) => {
                                    setInput((input:any)=>{
                                        if (e.updated_src) {
                                            const newInput = [...input]
                                            newInput[idx].value = e.updated_src
                                            console.log("NEW INPU==", newInput)
                                            return newInput
                                        }
                                        return input
                                    })
                                }}
                                onEdit={(e) => {
                                    console.log(e)
                                    setInput((input:any)=>{
                                        if (e.updated_src) {
                                            const newInput = [...input]
                                            newInput[idx].value = e.updated_src
                                            console.log("NEW INPU==", newInput)
                                            return newInput
                                        }
                                        return input
                                    })
                                }}
                            />
                            <FaDeleteLeft onClick={() => {
                                const newI = input.filter((i: any) => i.name !== item.name)
                                setInput([...newI])
                            }}
                                className="cursor-pointer"
                            />
                        </div>
                    ))
                }

            </div>
            <div onClick={handleRun} className="cursor-pointer flex items-center my-4">
                Run <FaPlay className="mx-2" /> <div>
                    {result.error
                        ? <div className="text-red-500">Error</div>
                        : result.tracking && result.tracking.tracks
                            ? <div className="text-yellow-500">Warning</div>
                            : result.output != undefined
                                ? <div className="text-green-500">Success</div>
                                : result.running
                                    ? <div className="text-blue-500">Running</div> : ""
                    }
                </div>
            </div>
            <div className="relative">
                <div
                    className={`absolute top-2 left-2 right-2 bottom-2 pointer-events-none z-0`}
                >
                    {editor.map((exp: any, idx: any) => (
                        <span key={idx}>
                            {
                                exp.map((e: any, edx: any) => (
                                    <span key={edx} className={`${e.className} whitespace-pre`}>{e.value}</span>
                                ))
                            }
                        </span>
                    ))}
                </div>
            </div>

            <textarea
                spellCheck="false"
                style={{
                    caretColor:"white",
                    resize: 'none',
                    backgroundColor: 'rgba(0, 0, 0, 0)',
                    borderColor: 'rgba(0, 0, 0, 0)',
                    color: 'rgba(0, 0, 0, 0)',
                }}
                className="w-11/12 h-full z-10 bg-transparent border-gray-500 border-2 p-2 min-h-[500px]"
                value={text}
                onChange={handleInputChange}
                onKeyDown={handleKeyDown}
            />
            <div>
                <div className="border-green-500 border-2 px-2 py-2 w-11/12 border-dashed mb-4">
                    <div className="font-bold">Output</div>
                    {
                        result.output && Array.isArray(result.output) && result.output.map((e:any, idx:any) =>(
                            <div key={idx} className="flex items-center">
                                <div className="w-12">{e.send}</div>
                                <div className="ml-10">
                                   {typeof e.msg == "object" ? <ReactJson src={e.msg} name={false}/> : e.msg} 
                                </div>
                                <FaArrowRight />
                                <div className="flex-col text-green-500 ">
                                    {
                                        e.reciever.split(",").map((i:any)=>(
                                            <div className="mt-2 " key={i}>{i}</div>
                                        ))
                                    }
                                </div>
                            </div>
                        ))
                    }
                </div>
                <div className="border-yellow-500 border-2 px-2 py-2 w-11/12 border-dashed mb-4">
                    <div className="font-bold">Tracking</div>
                    {
                        result.tracking && result.tracking.tracks &&<div>{result.tracking.tracks.map((e:any, idx:any)=>(
                            <div key={idx}>{JSON.stringify(e)}</div>
                        ))}</div>
                    }
                </div>
                <div className="border-red-500 border-2 px-2 py-2 w-11/12 border-dashed mb-4">
                    <div className="font-bold">Error</div>
                    {
                        result.error && <div className="text-red-500">{result.error}</div>
                    }
                </div>
            </div>
            <Modal
                open={openInput}
                onClose={() => { setOpenInput(false); setInName("") }}
                center
            >
                <Radio.Group onChange={(e) => { setInputType(e.target.value) }} value={inputType}>
                    <Radio value={1}>Task</Radio>
                    <Radio value={2}>Channel</Radio>
                </Radio.Group>
                <div className="mx-2">{inName}_{inputType == 1 ? 'task' : 'channel'}</div>
                <input placeholder="name"
                    autoFocus={true}
                    className="border-rounded mx-2 my-2 px-2 py-2"
                    onKeyDown={(e) => {
                        if (e.key == "Enter") {
                            setInput((input: any[]) => {
                                for (const i of input) {
                                    if (i.name == inName) {
                                        Toast.error(`var ${inName} existed`)
                                        return input
                                    }
                                }
                                let value: any = {
                                    result: {
                                        output: "",
                                        success: false,
                                        finish: false,
                                    }
                                }
                                if (inputType == 2) {
                                    value = {
                                        content: {}
                                    }
                                }
                                const root = `${inName}_${inputType == 1 ? 'task' : 'channel'}`
                                return [...input, { name: root, value }]

                            })
                            setOpenInput(false); setInName("")
                        }
                    }}
                    onChange={(e) => { setInName(e.target.value) }} />

            </Modal>
        </div>
    );
}

export default FuntionPage