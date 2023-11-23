import { useEffect, useRef, useState } from "react"
import { TrafficViewer } from "../components/TrafficViewer"
import { Entry } from "../models/entry"
import { useRecoilValue } from "recoil"
import workflowMonitorAtom from "../recoil/workflowMonitor/atom"
import { BrokerPoint, DeliverFlow, LogMessageFlow, RecieverFlow } from "../conts"

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
    const [lastUpdated, setLastUpdated] = useState(0)
    const wfCmd = useRecoilValue(workflowMonitorAtom)
    const cnt = useRef(0)
    const entriesRef = useRef<Entry[]>([])

    useEffect(() => {
        if (wfCmd && wfCmd.cmd == LogMessageFlow) {
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
                setEntries={setEntries}
                setLastUpdated={setLastUpdated}
            />
        </>
    )
}

export default DashBoardPage