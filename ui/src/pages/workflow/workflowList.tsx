import { useEffect, useState } from "react";
import { UploadButton } from "../../components/UploadButton"
import { SetStatusWorkflow, Workflow, WorkflowFile } from "../../models/workflow";
import { services } from "../../services";
import { WorkflowEditor } from "../../components/WorkflowEditor";
import { Drawer, Table } from "antd";
import { ColumnsType } from "antd/es/table";
import { useAsync } from "react-use";
import { Toast } from "../../components/Toast";
import { formatDate } from "../../helper/date";
import { BiSolidShow } from "react-icons/bi";
import { MdCreate, MdDelete } from "react-icons/md";
import { ObjectEditor } from "../../components/ObjectEditor";
import { IoIosAdd } from "react-icons/io";
import { Delivering, Pause, Pending, Running } from "../../conts";
import { useRecoilValue } from "recoil";
import workflowMonitorAtom from "../../recoil/workflowMonitor/atom";
import Modal from "react-responsive-modal";

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
    const wfCmd = useRecoilValue(workflowMonitorAtom)
    const onWfYamlClose = () => {
        setShowYamlEditor(false)
        setWorkflowFile(undefined)
    };

    const rowSelection = {
        onChange: (_: React.Key[], selectedRows: unknown[]) => {
            setWfSelected(selectedRows.length)
        },
    };

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
                    value === Pause
                        ? <div>Pause</div>
                        : value === Delivering
                            ? <div>Delivering</div>
                            : value === Running
                                ? <div>Running</div>
                                : value === Pending
                                    ? <div>Pending</div> : ""
                }</>
            }
        },
        {
            title: 'Name',
            dataIndex: 'name',
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
            dataIndex: 'created_at',
            render: (_: any, record: Workflow) => {
                return (
                    <div className="flex">
                        <MdDelete className="w-6 h-6 cursor-pointer"

                        />
                        <BiSolidShow className="w-6 h-6 cursor-pointer"
                            onClick={() => {
                                console.log(record)
                                setWorkflowFile(record.file)
                                setOnlyRead(true)
                                setShowYamlEditor(true)
                            }}
                        />
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
        if(e && e.err){
            setError(new Error(e.err))
            return
        }
        onWfYamlClose()
        setReload(e => e + 1)
    }

    useEffect(()=>{
        if(wfCmd &&wfCmd.cmd === SetStatusWorkflow){
            console.log({wfCmd})
            for(const w of workflows){
                if(w.id == wfCmd.workflow_id){
                    w.status = wfCmd.data.status
                }
            }
            setWorkflows([...workflows])
            console.log(workflows)
        }
    },[wfCmd])

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
                        <UploadButton onUpload={(e:Workflow)=>{
                            if(e){
                                setWorkflowFile(e)
                            }
                        }} onError={setError} />
                    </div>
                }
               </div>

            </Modal>

        </div>
    )
}

export default WorkflowListPage