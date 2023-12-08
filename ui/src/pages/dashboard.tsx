import { useEffect, useRef, useState } from "react"
import { TrafficViewer } from "../components/TrafficViewer"
import { Entry } from "../models/entry"
import { useRecoilValue } from "recoil"
import workflowMonitorAtom from "../recoil/workflowMonitor/atom"
import { BrokerPoint, DeliverFlow, LogMessageFlow, RecieverFlow } from "../conts"
import { useAsync } from "react-use"
import { services } from "../services"

// const Entries = [
//     {
//         id: 1,
//         status: 1,
//         timestamp: 1700390582,
//         src: {
//             id: 1,
//             name: "client",
//             type: 1,
//         },
//         dst: {
//             id: 2,
//             name: "broker",
//             type: 2,
//         },
//         outgoing: true,
//         requestSize: 10,
//         responseSize: 999,
//         elapsedTime: 1234,
//         workflow: {
//             name: "abc",
//             id: 1,
//         },
//         cmd: 1,
//         request: "",
//         response: "",
//     },
//     {
//         id: 2,
//         status: 2,
//         timestamp: 1700390582,
//         src: {
//             id: 1,
//             name: "broker",
//             type: 1,
//         },
//         dst: {
//             id: 2,
//             name: "client2",
//             type: 1,
//         },
//         outgoing: true,
//         requestSize: 10,
//         responseSize: 999,
//         elapsedTime: 1234,
//         workflow: {
//             name: "abc",
//             id: 1,
//         },
//         cmd: 1,
//         request: "",
//         response: "",
//     },
//     {
//         id: 3,
//         status: 0,
//         timestamp: 1700390582,
//         src: {
//             id: 1,
//             name: "broker",
//             type: 1,
//         },
//         dst: {
//             id: 2,
//             name: "client3",
//             type: 1,
//         },
//         outgoing: false,
//         requestSize: 10,
//         responseSize: 999,
//         elapsedTime: 1234,
//         workflow: {
//             name: "abc",
//             id: 1,
//         },
//         cmd: 1,
//         request: "",
//         response: "",
//     },
// ]
const DashBoardPage = () => {
    const [entries, setEntries] = useState<Entry[]>([])
    const wfCmd = useRecoilValue(workflowMonitorAtom)
    const cnt = useRef(0)
    const [view, setView] = useState(0);
    const entriesRef = useRef<Entry[]>([])

    useAsync(async () => {
        if (view) {
            const records = await services.records
                .getMessageRecord()
                .catch()
            console.log({records})
            const entries : Entry[] = []
            for(const record of records){
                console.log("RUN", record)
                cnt.current++;
                entries.push({
                    id: cnt.current,
                    flow_id: record.id,
                    status: record.status,
                    timestamp: record.created_at,
                    src: {
                        id: record.sender_id,
                        name: record.sender_name,
                        type: record.sender_type,
                    },
                    dst: {
                        id: record.receiver_id,
                        name: record.receiver_name,
                        type: record.receiver_type,
                    },
                    outgoing: false,
                    requestSize: record.request_size ? record.request_size : -1,
                    responseSize: record.response_size ? record.response_size : -1,
                    elapsedTime: record.elapsed_time,
                    workflow: {
                        name: wfCmd.workflow_name,
                        id: wfCmd.workflow_id,
                    },
                    cmd: record.cmd,
                    request: record.flow === DeliverFlow ? record.message ? JSON.parse(record.message):'' : '',
                    response: record.flow === RecieverFlow ?  record.message ? JSON.parse(record.message):'' : '',
                    reply: record.flow === RecieverFlow
                })
            }
            console.log({entries})
            setEntries(entries)
        }else{
            setEntries([])
        }

    }, [view])

    useEffect(() => {
        if (wfCmd && wfCmd.cmd == LogMessageFlow &&!view) {
            const data = wfCmd.data
            const from = data.from
            const receiver = data.receiver
            const msg = data.msg
            cnt.current++;
            console.log(cnt.current)
            const entry = {
                id: cnt.current,
                flow_id: data.id,
                status: msg.status,
                timestamp: msg.send_at,
                src: {
                    id: from.id,
                    name: from.name,
                    type: from.type,
                },
                dst: {
                    id: receiver.id,
                    name: receiver.name,
                    type: receiver.type,
                },
                outgoing: false,
                requestSize: data.request_size ? data.request_size : -1,
                responseSize: data.response_size ? data.response_size : -1,
                elapsedTime: data.elapsed_time,
                workflow: {
                    name: wfCmd.workflow_name,
                    id: wfCmd.workflow_id,
                },
                cmd: msg.cmd,
                request: data.flow === DeliverFlow ? data.msg : '',
                response: data.flow === RecieverFlow ? data.msg : '',
                reply: data.flow === RecieverFlow
            }

            if (entriesRef.current.length >= 50) {
                entries.pop();
                entries.unshift(entry)
            } else {
                entriesRef.current.push(entry)
                setEntries(entries => ([entry, ...entries]))
            }
            console.log("Dashboard", wfCmd)
        }
    }, [wfCmd])

    return (
        <>
            <TrafficViewer
                entries={entries}
                setView={setView}
            />
        </>
    )
}

export default DashBoardPage