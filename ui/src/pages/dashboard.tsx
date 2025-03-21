import { useEffect, useRef, useState } from "react"
import { TrafficViewer } from "../components/TrafficViewer"
import { Entry } from "../models/entry"
import { useRecoilValue } from "recoil"
import workflowMonitorAtom from "../recoil/workflowMonitor/atom"
import { BrokerPoint, DeliverFlow, LogMessageFlow, RecieverFlow, ViewHistory, ViewRealtime, ViewTimeLine } from "../conts"
import { useAsync } from "react-use"
import { services } from "../services"
import { useLocation, useNavigate, useParams } from "react-router-dom"
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
    const [value, setValue] = useState(0);
    const nav = useNavigate()
    const [offset, setOffset] = useState(0)

    useAsync(async () => {
        if (viewType == ViewHistory) {

            const records = await services.records
                .getMessageRecord(offset)
                .catch()
            const entries: Entry[] = []
            for (const record of records) {
                cnt.current++;
                var msg: any = ""
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
    }, [viewType, offset])

    const params = useParams()

    useEffect(() => {
        switch (params.view) {
            case "timeline":
                setViewType(ViewTimeLine)
                break;
            case "history":
                setViewType(ViewHistory)
                break;
            default:
                setViewType(ViewRealtime)

        }
    }, [params])

    useEffect(() => {
        switch (value) {
            case 1:
                nav("/dashboard/logs")
                break
            case 2:
                nav("/dashboard/timeline")
                break;
            case 3:
                nav("/dashboard/history")
                break;
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
            const record = wfCmd.data
            const entry = {
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
                    id: record.workflow_id,
                },
                cmd: record.cmd,
                payload: msg,
                reply: record.flow === RecieverFlow
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
            <div className="ml-6 flex items-center">
                <Radio.Group onChange={onChange} value={viewType}>
                    <Radio value={1}>Logs</Radio>
                    <Radio value={2}>Timeline</Radio>
                    <Radio value={3}>History</Radio>
                </Radio.Group>
            </div>
            <TrafficViewer
                entries={entries}
                viewType={viewType}
                setOffset={setOffset}
            />
        </>
    )
}

export default DashBoardPage