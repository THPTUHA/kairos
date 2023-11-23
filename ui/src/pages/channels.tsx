import Table, { ColumnsType } from "antd/es/table";
import { Channel } from "../models/channel";
import { formatDate } from "../helper/date";
import { useAsync } from "react-use";
import { services } from "../services";
import { useEffect, useState } from "react";
import { Toast } from "../components/Toast";
import { IoIosAdd } from "react-icons/io";
import Modal from "react-responsive-modal";
import { Input } from "antd";
import { RiWechatChannelsLine } from "react-icons/ri";

const locale = {
    emptyText: <span>Empty channel</span>,
};

const ChannelPage = () => {
    const [channelSelected, setChannelSelected] = useState(0)
    const [error, setError] = useState<Error>();
    const [reload, setReload] = useState(0)
    const [showModalForm, setShowModalForm] = useState(false)
    const [channelName, setChannelName] = useState("")

    const columns: ColumnsType<Channel> = [
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
    ];

    const channels = useAsync(async () => {
        const cs = await services.channels
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

    const createChannel = async (name: string) => {
        await services.channels
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
                    <span>Create channel</span>
                </span>
                {channelSelected > 0 ?
                    <span className="flex items-center">
                        <button className="bg-red-500 w-12 rounded ml-2 py-1">Delete</button>
                        <span>{channelSelected} selected</span>
                    </span>
                    : ""}
            </div>
            {
                channels.value ?
                    <Table
                        rowSelection={rowSelection}
                        columns={columns}
                        dataSource={channels.value}
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
                <Input size="large" placeholder="channel name" style={{ width: 500 }}
                    onKeyDown={(e) => {
                        if (e.code === "Enter") {
                            createChannel(channelName)
                        }
                    }}
                    onChange={(e) => { setChannelName(e.target.value) }}
                    prefix={<RiWechatChannelsLine />} />
            </Modal>
        </div>
    )
}

export default ChannelPage