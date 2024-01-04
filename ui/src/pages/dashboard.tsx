import { useEffect, useRef, useState } from "react"
import { TrafficViewer } from "../components/TrafficViewer"
import { Entry } from "../models/entry"
import { useRecoilValue } from "recoil"
import workflowMonitorAtom from "../recoil/workflowMonitor/atom"
import { BrokerPoint, DeliverFlow, LogMessageFlow, RecieverFlow, ViewHistory, ViewRealtime, ViewTimeLine } from "../conts"
import { useAsync } from "react-use"
import { services } from "../services"
import { useLocation, useNavigate } from "react-router-dom"
import { Checkbox, Radio, RadioChangeEvent } from "antd"

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
    const [viewType, setViewType] = useState(0);
    const entriesRef = useRef<Entry[]>([])
    const [value, setValue] = useState(1);
    const location = useLocation()
    const nav = useNavigate()
    const [offset, setOffset]  = useState(0)
    
    useAsync(async () => {
        if (viewType == ViewHistory) {

            const records = await services.records
                .getMessageRecord(offset)
                .catch()
            const entries: Entry[] = []
            for (const record of records) {
                cnt.current++;
                var msg :any= ""
                try {
                    msg = JSON.parse(record.message)
                    delete msg["workflow_id"]
                    delete msg["run_coun"]
                    delete msg["offset"]
                } catch (error) {
                    msg = record.message
                }
                try {
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
                            name: record.workflow_name,
                            id: record.workflow_id,
                        },
                        cmd: record.cmd,
                        payload: msg,
                        reply: record.flow === RecieverFlow
                    })
                } catch (error) {
                    console.log("ERR", error)
                }
            }
            setEntries(entries)
        } else {
            setEntries([])
        }
    }, [viewType,offset])

    useEffect(() => {
        if (location.pathname) {
            const q = location.search.split("&")[0]
            console.log("Path", q)
            if (!q) {
                setViewType(ViewRealtime)
            } else if (q == "?view=history") {
                setViewType(ViewHistory)
            } else if (q == "?view=timeline") {
                setViewType(ViewTimeLine)
            }
        }
    }, [location])

    useEffect(() => {
        if (value == 1) {
            nav("/dashboard")
        } else if (value == 2) {
            nav("/dashboard?view=timeline")
        } else if (value == 3) {
            nav("/dashboard?view=history")
        }
    }, [value])

    useEffect(() => {
        if (wfCmd && wfCmd.cmd == LogMessageFlow && (viewType == ViewRealtime || viewType == ViewTimeLine)) {
            cnt.current++;
            console.log(cnt.current)
            var msg = ""
            try {
                msg = JSON.parse(wfCmd.message)
            } catch (error) {
                msg = wfCmd.message
            }

            const entry = {
                id: cnt.current,
                flow_id: wfCmd.id,
                status: wfCmd.status,
                timestamp: wfCmd.send_at,
                src: {
                    id: wfCmd.sender_id,
                    name: wfCmd.sender_name,
                    type: wfCmd.sender_type,
                },
                dst: {
                    id: wfCmd.receiver_id,
                    name: wfCmd.receiver_name,
                    type: wfCmd.receiver_type,
                },
                outgoing: false,
                requestSize: wfCmd.request_size ? wfCmd.request_size : -1,
                responseSize: wfCmd.response_size ? wfCmd.response_size : -1,
                elapsedTime: 0,
                workflow: {
                    name: wfCmd.workflow_name,
                    id: wfCmd.workflow_id,
                },
                cmd: wfCmd.cmd,
                payload: msg,
                reply: wfCmd.flow === RecieverFlow
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
    }, [wfCmd, viewType])

    const onChange = (e: RadioChangeEvent) => {
        console.log('radio checked', e.target.value);
        setValue(e.target.value);
    };

    return (
        <>
            <Radio.Group onChange={onChange} value={value}>
                <Radio value={1}>Logs</Radio>
                <Radio value={2}>Timeline</Radio>
                <Radio value={3}>History</Radio>
            </Radio.Group>
            <TrafficViewer
                entries={entries}
                viewType={viewType}
                setOffset={setOffset}
            />
        </>
    )
}

export default DashBoardPage