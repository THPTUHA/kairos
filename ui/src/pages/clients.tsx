import { useEffect, useState } from "react"
import { IoIosAdd } from "react-icons/io"
import { useAsync } from "react-use"
import { services } from "../services"
import { Input, Table } from "antd"
import { ColumnsType } from "antd/es/table"
import { Client } from "../models/client"
import { formatDate } from "../helper/date"
import { Toast } from "../components/Toast"
import { GrSystem } from "react-icons/gr"
import Modal from "react-responsive-modal"

const locale = {
    emptyText: <span>Empty client</span>,
};

const ClientPage = () => {
    const [clientSelected, setClientSelected] = useState(0)
    const [error, setError] = useState<Error>();
    const [showModalForm, setShowModalForm] = useState(false)
    const [clientName, setClientName] = useState("")
    const [reload, setReload] = useState(0)

    const clients = useAsync(async () => {
        const ws = await services.clients
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

    const columns: ColumnsType<Client> = [
        {
            title: 'Status',
            dataIndex: 'stauts',
            width: 20,
            render: (value: number) => {
                return <>{
                    value === 0 ? <div>Stop</div> : <div>Running</div>
                }</>
            }
        },
        {
            title: 'Name',
            dataIndex: 'name',
        },
        {
            title: 'Created At',
            dataIndex: 'created_at',
            render: (value: number) => {
                return <>{formatDate(value)}</>
            }
        },
        {
            title: 'Active since',
            dataIndex: 'active_since',
            render: (value: number) => {
                return <>{!value ?"No active" :formatDate(value)}</>
            }
        },
    ];

    const createClient = async (name: string) => {
        const client = await services.clients
            .create({
                name: name
            })
            .catch(setError)
        setReload(reload => reload + 1)
        setShowModalForm(false)
    }

    return (
        <div>
            <div className="flex">
                <span className="flex items-center bg-blue-500 w-40 justify-center rounded py-1 cursor-pointer"
                    onClick={() => {
                        setShowModalForm(true)
                    }}
                >
                    <IoIosAdd className="w-6 h-6" />
                    <span>Create client</span>
                </span>
                {clientSelected > 0 ?
                    <span className="flex items-center">
                        <button className="bg-red-500 w-12 rounded ml-2 py-1">Delete</button>
                        <span>{clientSelected} selected</span>
                    </span>
                    : ""}
            </div>
            {
                clients.value ?
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={clients.value}
                        locale={locale}
                    /> : <div>Loading</div>
            }

            <Modal
                open={showModalForm}
                onClose={() => {
                    setShowModalForm(false)
                }}
                classNames={{
                    closeIcon: ""
                }}
                center
            >
                <Input size="large" placeholder="client name" style={{ width: 500 }}
                    onKeyDown={(e) => {
                        if (e.code === "Enter") {
                            createClient(clientName)
                        }
                    }}
                    onChange={(e) => { setClientName(e.target.value) }}
                    prefix={<GrSystem />} />
            </Modal>
        </div>
    )
}

export default ClientPage