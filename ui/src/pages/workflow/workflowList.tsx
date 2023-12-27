import { useEffect, useState } from "react";
import { UploadButton } from "../../components/UploadButton"
import { DestroyWorkflow, SetStatusWorkflow, Workflow, WorkflowFile } from "../../models/workflow";
import { services } from "../../services";
import { WorkflowEditor } from "../../components/WorkflowEditor";
import { Drawer, Popconfirm, Table, Tooltip } from "antd";
import { ColumnsType } from "antd/es/table";
import { useAsync } from "react-use";
import { Toast } from "../../components/Toast";
import { formatDate } from "../../helper/date";
import { BiSolidShow } from "react-icons/bi";
import { MdCreate, MdDelete, MdOutlineSevereCold } from "react-icons/md";
import { ObjectEditor } from "../../components/ObjectEditor";
import { IoIosAdd } from "react-icons/io";
import { Delivering, Pause, Pending, Running, Destroying, Recovering, RecoverWorkflow } from "../../conts";
import { useRecoilState, useRecoilValue } from "recoil";
import workflowMonitorAtom from "../../recoil/workflowMonitor/atom";
import Modal from "react-responsive-modal";
import { RiDeviceRecoverLine } from "react-icons/ri";
import { useNavigate } from "react-router-dom";

const locale = {
    emptyText: <span>Empty workflow</span>,
};

const WorklowFileDefault: WorkflowFile = {
    version: "1.0"
}

const WorkflowListPage = () => {
    const [workflowFile, setWorkflowFile] = useState<WorkflowFile>();
    const [wfSelected, setWfSelected] = useState(0)
    const [onlyRead, setOnlyRead] = useState(false)
    const [error, setError] = useState<Error>();
    const [showYamlEditor, setShowYamlEditor] = useState(false)
    const [reload, setReload] = useState(0)
    const [workflows, setWorkflows] = useState<Workflow[]>([])
    const [openLog, setOpenLog] = useState(0)

    const [wfCmd, setWfCmd] = useRecoilState(workflowMonitorAtom)
    const nav = useNavigate()

    const onWfYamlClose = () => {
        setShowYamlEditor(false)
        setWorkflowFile(undefined)
    };

    const rowSelection = {
        onChange: (_: React.Key[], selectedRows: unknown[]) => {
            setWfSelected(selectedRows.length)
        },
    };

    const wflog = useAsync(async () => {
        if (openLog) {
            const ws = await services.workflows
                .record(openLog)
                .catch(setError)
            if (Array.isArray(ws)) {
                return ws
            }
        }
        return []
    }, [openLog])

    const wff = useAsync(async () => {
        const ws = await services.workflows
            .list()
            .catch(setError)
        if (Array.isArray(ws)) {
            setWorkflows(ws)
            return ws
        }
        return []
    }, [reload])

    const columns: ColumnsType<Workflow> = [
        {
            title: 'Status',
            dataIndex: 'status',
            width: 20,
            render: (value: number) => {
                return <>{
                    value == Pause
                        ? <div>Pause</div>
                        : value == Delivering
                            ? <div>Delivering</div>
                            : value == Running
                                ? <div>Running</div>
                                : value == Pending
                                    ? <div>Pending</div> :
                                    value == Destroying
                                        ? <div>Destroying</div> : 
                                        value == Recovering
                                        ? <div>Recovering</div>:""
                }</>
            }
        },
        {
            title: 'Name',
            dataIndex: 'name',
            render: (value: string)=>{
                return <div 
                className="cursor-pointer"
                onClick={()=>{
                    nav("/dashboard?view=timeline")
                }}>{value}</div>
            }
        },
        {
            title: 'Namespace',
            dataIndex: 'namespace',
        },
        {
            title: 'Created At',
            dataIndex: 'created_at',
            render: (value: number) => {
                return <>{formatDate(value)}</>
            }
        },
        {
            title: 'Action',
            dataIndex: 'action',
            render: (_: any, record: Workflow) => {
                return (
                    <div className="flex">
                        <Popconfirm
                            title={"Destroy worklow"}
                            onConfirm={() => {
                                services.workflows.drop(record.id).then().catch()
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
                                    console.log(record)
                                    setWorkflowFile(record.file)
                                    setOnlyRead(true)
                                    setShowYamlEditor(true)
                                }}
                            />
                        </Tooltip>
                        <Tooltip title={"log"}>
                            <MdOutlineSevereCold className="w-6 h-6 cursor-pointer"
                                onClick={() => {
                                    setOpenLog(record.id)
                                }}
                            />
                        </Tooltip>
                        <Tooltip title={"recover"}>
                            <RiDeviceRecoverLine
                                className="w-6 h-6 cursor-pointer"
                                onClick={() => {
                                    services.workflows
                                        .recover(record.id)
                                        .then(()=>{
                                            Toast.success("Start recover")
                                        })
                                        .catch(setError)
                                }}
                            />
                        </Tooltip>
                    </div>
                )
            }
        },
    ];

    useEffect(() => {
        if (error) {
            console.log("ERROR", error)
            Toast.error(error.message)
        }
    }, [error])

    function onCreateWorkflow(e: any) {
        if (e && e.err) {
            setError(new Error(e.err))
            return
        }
        onWfYamlClose()
        setReload(e => e + 1)
    }

    useEffect(() => {
        if (wfCmd) {
            if (wfCmd.cmd === SetStatusWorkflow) {
                for (const w of workflows) {
                    if (w.id == wfCmd.workflow_id) {
                        w.status = wfCmd.data.status
                    }
                }
                setWorkflows([...workflows])
                console.log(workflows)
            }
            if (wfCmd.cmd === DestroyWorkflow) {
                const wfs = workflows.filter(item => item.id != wfCmd.workflow_id)
                setWorkflows(wfs)
            }

            if(wfCmd.cmd === RecoverWorkflow){
                if(wfCmd.data.status === "false"){
                    Toast.error("Workflow recover error")
                }else if(wfCmd.data.status === "true"){
                    Toast.success("Workflow recover success")
                }
                setWfCmd(null)
            }
        }
    }, [wfCmd])

    return (
        <div>
            <div className="flex">
                <span className="flex items-center bg-blue-500 w-40 justify-center rounded py-1 cursor-pointer"
                    onClick={() => {
                        setOnlyRead(false)
                        setShowYamlEditor(true)
                    }}
                >
                    <IoIosAdd className="w-6 h-6" />
                    <span>Create workflow</span>
                </span>
                {wfSelected > 0 ?
                    <span className="flex items-center">
                        <button className="bg-red-500 w-12 rounded ml-2 py-1">Delete</button>
                        <span>{wfSelected} selected</span>
                    </span>
                    : ""}
            </div>
            {
                wff.value ?
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={workflows}
                        locale={locale}
                    /> : <div>Loading</div>
            }

            <Modal
                open={showYamlEditor}
                onClose={onWfYamlClose}
                center
            >
                <div className="min-w-[800px]">
                    {
                        workflowFile && onlyRead && <ObjectEditor value={workflowFile} />
                    }

                    {
                        !onlyRead && <WorkflowEditor template={workflowFile ? workflowFile : WorklowFileDefault} onChange={setWorkflowFile} />
                    }
                    {
                        !onlyRead &&
                        <div className="flex">
                            <button onClick={() => {
                                if (workflowFile) {
                                    services.workflows
                                        .create(workflowFile)
                                        .then(onCreateWorkflow)
                                        .catch(setError);
                                }
                            }}
                                className="bg-green-500"
                            > Apply</button>
                            <UploadButton onUpload={(e: Workflow) => {
                                if (e) {
                                    setWorkflowFile(e)
                                }
                            }} onError={setError} />
                        </div>
                    }
                </div>
            </Modal>
            <Modal
                open={openLog > 0}
                onClose={() => { setOpenLog(0) }}
                center
            >
                <div className="min-w-[400px]" >
                    {
                        wflog.loading ? <div>Loading </div> :
                            <div >{wflog.value?.map(v => (
                                <div key={v.id}>
                                    <div className={`${v.status == 1 ? "bg-green-500" : "bg-red-500"} flex`}>
                                        <div>{v.status == 1 ? "Success" : "Fault"}</div>
                                        <div>{formatDate(v.created_at)}</div>
                                    </div>
                                    <div>{v.record}</div>
                                </div>
                            ))}</div>
                    }
                </div>
            </Modal>
        </div>
    )
}

export default WorkflowListPage