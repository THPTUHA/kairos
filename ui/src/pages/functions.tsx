import { useEffect, useState } from "react"
import { IoIosAdd } from "react-icons/io"
import { useAsync } from "react-use"
import { services } from "../services"
import { Input, Table } from "antd"
import { ColumnGroupType, ColumnsType } from "antd/es/table"
import { formatDate } from "../helper/date"
import { Toast } from "../components/Toast"
import Modal from "react-responsive-modal"
import { Function } from "../models/function"
import { UploadButton } from "../components/UploadButton"
import CodeEditor from '@uiw/react-textarea-code-editor';
import { MdDelete } from "react-icons/md"
import { BiSolidShow } from "react-icons/bi"

const locale = {
    emptyText: <span>Empty function</span>,
};

function checkAndGetFunction(code:string) {
    const functionRegex = /function\s*([a-zA-Z_$][0-9a-zA-Z_$]*)\s*\([^)]*\)\s*\{[^]*\}/;
    const matches = code.match(functionRegex);
    if (matches) {
        if(matches.length > 2){
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
    const [reload, setReload] = useState(0)
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
                        <MdDelete className="w-6 h-6 cursor-pointer"

                        />
                        <BiSolidShow className="w-6 h-6 cursor-pointer"
                            onClick={() => {
                                console.log(record)
                                setCode(record.content)
                                setShowModalForm(true)
                            }}
                        />
                    </div>
                )
            }
        },
    ];


    function onCreateFuntion(e: any) {
        console.log(e)
        if(e && e.err){
            Toast.error(e.err)
            return
        }
        setReload(e => e + 1)
        setShowModalForm(false)
    }

    function createFunction() {
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
                    <span>Create func</span>
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
                        onChange={(evn) => setCode(evn.target.value)}
                        padding={15}
                        style={{
                            backgroundColor: "#f5f5f5",
                            fontFamily: 'ui-monospace,SFMono-Regular,SF Mono,Consolas,Liberation Mono,Menlo,monospace',
                        }}
                    />
                    <div className="flex">
                        <button onClick={createFunction}
                            className="bg-green-500"
                        > Apply</button>
                        <UploadButton onUpload={setCode} onError={setError} />
                    </div>
                </div>

            </Modal>
        </div>
    )
}

export default FuntionPage