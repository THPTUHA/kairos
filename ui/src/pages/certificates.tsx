import Table, { ColumnsType } from "antd/es/table";
import { formatDate } from "../helper/date";
import { useAsync } from "react-use";
import { services } from "../services";
import { useEffect, useState } from "react";
import { Toast } from "../components/Toast";
import { IoIosAdd } from "react-icons/io";
import Modal from "react-responsive-modal";
import { Checkbox, Input } from "antd";
import { Certificate } from "../models/certificate";
import { GrCertificate } from "react-icons/gr";
import { FaRegCopy } from "react-icons/fa6";

const locale = {
    emptyText: <span>Empty certificate</span>,
};

const Read = 1;
const Write = 2;
const ReadWrite = 3;

const CertificatePage = () => {
    const [channelSelected, setChannelSelected] = useState(0)
    const [error, setError] = useState<Error>();
    const [reload, setReload] = useState(0)
    const [showModalForm, setShowModalForm] = useState(false)
    const [certName, setCertName] = useState("")
    const [pers, setPers] = useState<any[]>([])
    const columns: ColumnsType<Certificate> = [
        {
            title: 'Name',
            dataIndex: 'name',
        },
        {
            title: 'Api key',
            dataIndex: 'api_key',
            ellipsis: true,
            render: (value: string) => {
                return <><FaRegCopy
                    className="cursor-pointer"
                    onClick={() => { navigator.clipboard && navigator.clipboard.writeText(value) }} />
                    {value}</>
            }
        },
        {
            title: 'Secret key',
            dataIndex: 'secret_key',
            ellipsis: true,
            render: (value: string) => {
                return <><FaRegCopy
                    className="cursor-pointer"
                    onClick={() => { navigator.clipboard&& navigator.clipboard.writeText(value) }} />
                    {value}</>
            }
        },
        {
            title: 'Created at',
            dataIndex: 'created_at',
            render: (value: number) => {
                return <>{formatDate(value)}</>
            }
        },
    ];

    const certs = useAsync(async () => {
        const cs = await services.certs
            .list()
            .catch(setError)
        if (Array.isArray(cs)) {
            return cs
        }
        return []
    }, [reload])

    useEffect(() => {
        if (error) {
            console.log("ERROR", error)
            Toast.error(error.message)
        }
    }, [error])

    const rowSelection = {
        onChange: (_: React.Key[], selectedRows: unknown[]) => {
            setChannelSelected(selectedRows.length)
        },
    };

    const channels = useAsync(async () => {
        const cs = await services.channels
            .list()
            .catch(setError)
        if (Array.isArray(cs)) {
            return cs
        }
        return []
    }, [reload])

    const chooseChannelRole = (channel_id: number, role: number) => {
        let isHas
        const npers = pers.map(item => {
            if (item.channel_id == channel_id) {
                isHas = true
                return {
                    channel_id,
                    role
                }
            }
            return item
        })
        if (!isHas) {
            setPers([...npers, { channel_id, role }])
        } else {
            console.log(npers)
            setPers(npers)
        }
    }

    const getRoleChannel = (channel_id: number) => {
        for (const p of pers) {
            if (p.channel_id === channel_id) {
                return p.role
            }
        }
        return 0
    }

    const createCert = async () => {
        let data = {
            name: certName,
            channel_permissions: pers
        }

        const cs = await services.certs
            .create(data)
            .catch(setError)
        setShowModalForm(false)
        setReload(reload => reload + 1)
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
                    <span className="text-white">Create certificate</span>
                </span>
                {channelSelected > 0 ?
                    <span className="flex items-center">
                        <button className="bg-red-500 w-12 rounded ml-2 py-1">Delete</button>
                        <span>{channelSelected} selected</span>
                    </span>
                    : ""}
            </div>
            {
                certs.value ?
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={certs.value}
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
                <Input size="large" placeholder="cert name" style={{ width: 500 }}
                    onChange={(e) => { setCertName(e.target.value) }}
                    prefix={<GrCertificate />} />
                <div>Channel</div>
                <div>
                    {
                        channels.value &&
                        <div>
                            {
                                channels.value.map(item => (
                                    <div className="flex" key={item.id}>
                                        <div className="w-1/3">{item.name}</div>
                                        <div className="w-2/3 flex justify-between">
                                            <div className="flex" onClick={() => { chooseChannelRole(item.id, Read) }}>
                                                <Checkbox checked={getRoleChannel(item.id) == Read} />
                                                <span>Read</span>
                                            </div>
                                            <div className="flex" onClick={() => { chooseChannelRole(item.id, Write) }}>
                                                <Checkbox checked={getRoleChannel(item.id) == Write} />
                                                <span>Write</span>
                                            </div>
                                            <div className="flex" onClick={() => { chooseChannelRole(item.id, ReadWrite) }}>
                                                <Checkbox checked={getRoleChannel(item.id) == ReadWrite} />
                                                <span>Read&Write</span>
                                            </div>
                                        </div>
                                    </div>
                                ))
                            }
                        </div>
                    }
                </div>
                <div className="py-2">
                    <button className="bg-green-500 px-2 mx-2 rounded"
                        onClick={createCert}
                    >Create</button>
                    <button className="bg-red-500 px-2 mx-2 rounded"
                        onClick={() => { setShowModalForm(false) }}
                    >Cannel</button>
                </div>
            </Modal>
        </div>
    )
}

export default CertificatePage