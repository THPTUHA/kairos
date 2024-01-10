import { useEffect, useRef, useState } from "react";
import ForceGraph from "../libs/ForceGraph"
import { useAsync } from "react-use";
import { services } from "../services";
import { Checkbox, Drawer, Menu, Radio, RadioChangeEvent, Tabs } from "antd";
import { CheckboxValueType } from "antd/es/checkbox/Group";
import { BrokerPoint, ChannelPoint, ClientPoint, ColorGreen, ColorRed, TaskPoint, TriggerStatusWorkflow } from "../conts";
import { formatDate } from "../helper/date";
import { FaChevronDown, FaChevronUp, FaDeleteLeft, FaRegClock } from "react-icons/fa6";
import ReactJson from "react-json-view";
import { parseBrokerFlows, reset } from "../helper/base";
import Modal from "react-responsive-modal";
import { Broker } from "../models/broker";
import { TiFlowSwitch } from "react-icons/ti";
import { Task } from "kairos-js";
import { Channel } from "../models/channel";
import { Toast } from "../components/Toast";
import { IoMdAddCircleOutline } from "react-icons/io";
import { GrTrigger } from "react-icons/gr";
import { Trigger } from "../models/trigger";
import { Cron } from 'react-js-cron'
import { MdOutlineAccessTime, MdOutlineSwitchAccessShortcut } from "react-icons/md";
import workflowMonitorAtom from "../recoil/workflowMonitor/atom";
import { useRecoilValue } from "recoil";
import { Link } from "react-router-dom";
interface Edge {
    id: string;
    from: string;
    to: string;
    value: number;
    count: number;
    cumulative: number;
    label: string;
    filter: string;
    proto: string;
    title?: string;
    color?: string;
}

interface Node {
    id: string;
    value: number;
    label: string;
    group: string;
    title?: string;
    color?: string;
    name?: string;
    namespace?: string;
    verb?: string;
}

interface GraphData {
    nodes: Node[];
    edges: Edge[];
}

// const data: GraphData = {
//     nodes: [
//         { id: 1, label: "hello", group: "1", value: 1, color: "red" },
//         { id: 2, label: "hello2", group: "1", value: 1 },
//         { id: 3, label: "extend", group: "2", value: 1, color: "blue" },
//     ],
//     edges: [
//         { id: 1, from: 1, to: 2, value: 12, count: 10, cumulative: 123, label: "vc", filter: "", proto: "" },
//         { id: 2, from: 1, to: 2, value: 12, count: 10, cumulative: 123, label: "vc2", filter: "", proto: "" },
//         { id: 3, from: 2, to: 3, value: 12, count: 10, cumulative: 123, label: "ok", filter: "", proto: "" },
//     ]
// }

function getTypeUser(type: number) {
    switch (type) {
        case ClientPoint:
            return "client"
        case ChannelPoint:
            return "channel"
        case BrokerPoint:
            return "broker"
        case TaskPoint:
            return "task"
    }
    return ""
}

const GraphPage = () => {
    const modalRef = useRef(null);
    const [selectedEdges, setSelectedEdges] = useState<string[]>([]);
    const [selectedNodes, setSelectedNodes] = useState<string[]>([]);
    const [graphOptions, setGraphOptions] = useState(ServiceMapOptions);
    const [graphData, setGraphData] = useState<GraphData>({
        nodes: [], edges: []
    });
    const [tasks, setTasks] = useState<any[]>([])
    const [clients, setClients] = useState<any[]>([])
    const [channels, setChannels] = useState<any[]>([])
    const [brokers, setBrokers] = useState<any[]>([])

    const [graphType, setGraphType] = useState(1)
    const [wfIDs, setWfIDs] = useState<any[]>([])
    const [error, setError] = useState<Error>();
    const [object, setObject] = useState<any>(null)

    const events = {
        select: ({ nodes, edges }: { nodes: string[], edges: string[] }) => {
            setSelectedEdges(edges);
            setSelectedNodes(nodes);
        }
    }

    const pointRecord = useAsync(async () => {
        if (selectedNodes.length == 0) {
            return {
                records: [],
                obj: {},
            }
        }
        const node = selectedNodes[0].split("-")
        const node_id = node[0]
        const node_type = node[1]
        if (node_type === "client") {
            const result = await services.records
                .getClientRecord(parseInt(node_id))
                .catch(setError)
            for (const r of result.client_records) {
                r.type = "client"
                for (const client of clients) {
                    if (client.id === r.client_id) {
                        r.name = client.name
                        break
                    }
                }

                for (const task of tasks) {
                    if (task.id === r.task_id) {
                        r.task_name = task.name
                        break
                    }
                }
            }
            return {
                records: result.client_records as [],
                obj: result.client
            }
        }

        if (node_type === "broker") {
            const result = await services.records
                .getBrokerRecord(parseInt(node_id))
                .catch(setError)
            console.log({ result })
            for (const r of result.broker_records) {
                r.type = "broker"
                for (const broker of brokers) {
                    if (broker.id === r.broker_id) {
                        r.name = broker.name
                        break
                    }
                }
            }


            return {
                records: result.broker_records as [],
                obj: result.broker,
            }
        }

        if (node_type === "task") {
            const result = await services.records
                .getTaskRecord(parseInt(node_id))
                .catch(setError)
            for (const r of result.task_records) {
                r.type = "task"
                for (const task of tasks) {
                    if (task.id === r.task_id) {
                        r.name = task.name
                        break
                    }
                }
            }
            return {
                records: result.task_records as [],
                obj: result.task,
            }
        }
        return {
            records: [],
            obj: {},
        }

    }, [selectedNodes])

    useEffect(() => {
        if (selectedNodes.length) {
            const sn = selectedNodes[0]
            const items = sn.split("-")
            const type = items[items.length - 1]
            const oid = items[0]
            console.log({ sn, type, oid, channels })
            let client = ""
            if (items.length == 3) {
                client = items[1]
            }
            if (type == "task") {
                for (const t of tasks) {
                    if (t.id == oid) {
                        setObject({
                            type: type,
                            obj: t,
                            client
                        })
                        break;
                    }
                }
            } else if (type == "broker") {
                for (const b of brokers) {
                    if (b.id == oid) {
                        setObject({
                            type: type,
                            obj: b,
                            client
                        })
                        break;
                    }
                }
            } else if (type == "channel") {
                for (const c of channels) {
                    if (c.id == oid) {
                        setObject({
                            type: type,
                            obj: c,
                        })
                        break;
                    }
                }
            }
        }
    }, [selectedNodes])

    const wfs = useAsync(async () => {
        const wfs = await services.workflows
            .list()
            .catch(setError)
        if (Array.isArray(wfs)) {
            return wfs
        }
        return []
    }, [])

    function onCloseDetail() {
        setSelectedEdges([]);
        setSelectedNodes([]);
    }

    useAsync(async () => {
        if (wfIDs.length == 0) {
            return []
        }
        const graph = await services.graphs
            .get({
                workflow_ids: wfIDs.map(id => parseInt(id))
            })
            .catch(setError)

        // const graphData = await services.graphs
        //     .data({
        //         workflow_ids: wfIDs.map(id => parseInt(id))
        //     })
        //     .catch(setError)
        const tasks = []
        const brokers = []
        const channels = []
        if (Array.isArray(graph)) {
            var nodes: Node[] = []
            var edges: Edge[] = []
            for (const g of graph) {
                // if (g.clients) {
                //     for (const client of g.clients) {
                //         clients.push(client)
                //         let exist = false
                //         for (const n of nodes) {
                //             if (n.id === `${client.id}-client`) {
                //                 exist = true
                //             }
                //         }
                //         if (!exist) {
                //             nodes.push({
                //                 id: `${client.id}-client`, label: client.name, group: g.id, value: 10, color: "blue"
                //             })
                //         }
                //     }
                // }
                if (g.channels) {
                    for (const channel of g.channels) {
                        let exist = false
                        for (const n of nodes) {
                            if (n.id === `${channel.id}-channel`) {
                                exist = true
                            }
                        }
                        if (!exist) {
                            nodes.push({
                                id: `${channel.id}-channel`, label: channel.name, group: g.id, value: 10, color: "white"
                            })
                        }
                        channels.push(channel)
                    }
                }

                if (g.tasks) {
                    const keys = Object.keys(g.tasks)
                    for (const key of keys) {
                        const task = g.tasks[key]
                        tasks.push(task)

                        if (task.clients) {
                            for (const c of task.clients) {
                                // if (g.clients) {
                                //     for (const client of g.clients) {
                                //         if (client.name === c) {
                                //             edges.push({
                                //                 id: `${task.id}-task#${client.id}-client`, from: `${task.id}-task`, to: `${client.id}-client`, value: 10, count: 10, cumulative: 123, label: "run", filter: "", proto: "",
                                //             })
                                //         }
                                //     }
                                // }
                                nodes.push({
                                    id: `${task.id}-${c}-task`, label: `${task.name}-${c}`, group: g.id, value: 10, color: "pink"
                                })
                            }
                        } else {
                            nodes.push({
                                id: `${task.id}-task`, label: `${task.name}`, group: g.id, value: 10, color: "pink"
                            })
                        }
                    }
                }

                if (g.brokers) {
                    const keys = Object.keys(g.brokers)
                    for (const key of keys) {
                        const broker = g.brokers[key]
                        brokers.push(broker)
                        try {
                            const arrs = reset(broker.flows)
                            // console.log("???",arrs)
                            const sends: string[] = []
                            for (const a of arrs) {
                                for (const e of a) {
                                    if (e.hasSend) {
                                        const eles = e.value.split(",")
                                        for (const x of eles) {
                                            if ((x.endsWith("_task") || x.endsWith("_channel")) && !sends.includes(x)) {
                                                sends.push(x.startsWith(".") ? x.slice(1) : x)
                                            }
                                        }
                                    }
                                }
                            }
                            broker.sends = sends
                        } catch (error) {
                            console.error(error)
                        }

                        if (broker.clients) {
                            for (const c of broker.clients) {
                                nodes.push({
                                    id: `${broker.id}-${c}-broker`, label: `${broker.name}-${c}`, group: g.id, value: 10, color: "#FFA500",
                                })
                            }
                        } else {
                            nodes.push({
                                id: `${broker.id}-broker`, label: `${broker.name}`, group: g.id, value: 10, color: "#FFA500",
                            })
                        }

                        for (const l of broker.listens) {
                            // for (const client of g.clients) {
                            //     if (l == `${client.name}_client`) {
                            //         edges.push({
                            //             id: `${client.id}-client#${broker.id}-broker`, from: `${client.id}-client`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "listen", filter: "", proto: "", color: "#FFA500"
                            //         })
                            //     }
                            // }

                            if (g.tasks) {
                                const keys = Object.keys(g.tasks)
                                for (const key of keys) {
                                    const task = g.tasks[key]
                                    if (l == `${task.name}_task`) {
                                        if (broker.clients) {
                                            for (const c of broker.clients) {
                                                edges.push({
                                                    id: `${task.id}-${c}-task#${broker.id}-${c}-broker`, from: `${task.id}-${c}-task`, to: `${broker.id}-${c}-broker`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "green"
                                                })
                                            }
                                        }
                                        if (!broker.client) {
                                            for (const c of task.clients) {
                                                edges.push({
                                                    id: `${task.id}-${c}-task#${broker.id}-broker`, from: `${task.id}-${c}-task`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "green"
                                                })
                                            }
                                        }
                                    }
                                }
                            }

                            for (const channel of g.channels) {
                                if (l == `${channel.name}_channel`) {
                                    edges.push({
                                        id: `${channel.id}-channel#${broker.id}-broker`, from: `${channel.id}-channel`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "green"
                                    })
                                }
                            }
                        }

                        for (const s of broker.sends) {
                            if (g.tasks) {
                                const keys = Object.keys(g.tasks)
                                for (const key of keys) {
                                    const task = g.tasks[key]
                                    if (s == `${task.name}_task`) {
                                        if (broker.clients) {
                                            for (const c of broker.clients) {
                                                edges.push({
                                                    id: `${broker.id}-${c}-broker#${task.id}-${c}-task`, from: `${broker.id}-${c}-broker`, to: `${task.id}-${c}-task`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "blue"
                                                })
                                            }
                                        }
                                        if (!broker.client) {
                                            for (const c of task.clients) {
                                                edges.push({
                                                    id: `${broker.id}-broker#${task.id}-${c}-task`, from: `${broker.id}-broker`, to: `${task.id}-${c}-task`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "blue"
                                                })
                                            }
                                        }
                                    }
                                }
                            }

                            for (const channel of g.channels) {
                                if (s == `${channel.name}_channel`) {
                                    edges.push({
                                        id: `${broker.id}-broker#${channel.id}-channel`, from: `${broker.id}-broker`, to: `${channel.id}-channel`, value: 10, count: 10, cumulative: 123, label: "", filter: "", proto: "", color: "blue"
                                    })
                                }
                            }
                        }
                    }
                }
            }
            console.log({ edges, nodes })
            const edgesNew: Edge[] = []
            // for (const data of graphData) {
            //     var send_type = getTypeUser(data.sender_type)
            //     var receiver_type = getTypeUser(data.receiver_type)
            //     var value = 10
            //     var label = "listen"
            //     switch (graphType) {
            //         case 2:
            //             value = data.count
            //             label = `${value}`
            //             break;
            //         case 3:
            //             value = Math.round(data.elapsed_time_avg)
            //             label = `${value} ms`
            //             break;
            //         case 4:
            //             value = Math.round(data.throughput)
            //             label = `${Utils.humanReadableBytes(value)}`
            //             break;
            //     }
            //     edgesNew.push({
            //         id: `${data.sender_id}-${send_type}#${data.receiver_id}-${receiver_type}@${data.workflow_id}`, from: `${data.sender_id}-${send_type}`, to: `${data.receiver_id}-${receiver_type}`, value: value, count: 10, cumulative: 123, label: label, filter: "", proto: ""
            //     })
            // }
            setTasks(tasks)
            setClients(clients)
            setBrokers(brokers)
            setChannels(channels)
            setGraphData({
                nodes: nodes,
                edges: graphType === 1 ? edges : edgesNew,
            })
            return graph
        }

        return []
    }, [wfIDs, graphType])

    function onSelectWorkflow(e: RadioChangeEvent) {
        setWfIDs([e.target.value])
    }

    console.log({ object })
    // function onChangeTypeGraph(e: RadioChangeEvent) {
    //     setGraphType(e.target.value)
    // }

    return (
        <div className="relative">
            <div className="w-1/3  absolute bg-gray-500 rounded px-2 py-2">
                <div>Workflow</div>
                {
                    wfs.loading ? <div>Loading</div> : (
                        <Radio.Group onChange={onSelectWorkflow} value={wfIDs[0]}>
                            {
                                wfs.value && wfs.value.map(item => (
                                    <Radio value={item.id} key={item.id}>{item.name}</Radio>
                                ))
                            }
                        </Radio.Group>
                    )
                }
                {/* <div>Type</div> */}

            </div>
            <div ref={modalRef} style={{ width: "100%", height: "100vh" }}>
                <ForceGraph
                    graph={graphData}
                    options={graphOptions}
                    events={events}
                    modalRef={modalRef}
                    setSelectedNodes={setSelectedNodes}
                    setSelectedEdges={setSelectedEdges}
                    selection={{
                        nodes: selectedNodes,
                        edges: selectedEdges,
                    }}
                />
            </div>
            <Modal
                open={object}
                onClose={() => { setObject(null); setSelectedNodes([]) }}
                center
            >
                {
                    object && <div className="min-w-[800px]">
                        {
                            object.type == "task" ?
                                <TaskDetail task={object.obj} client={object.client} wid={wfIDs[0]} />
                                : object.type == "broker" ?
                                    <BrokerDetail broker={object.obj} client={object.client} wid={wfIDs[0]} />
                                    : object.type == "channel" ?
                                        <ChannelDetail channel={object.obj} wid={wfIDs[0]} />
                                        : ''
                        }
                    </div>
                }
            </Modal>

            {/* <Drawer title="Record" placement="right" onClose={onCloseDetail} open={selectedNodes.length > 0 || selectedEdges.length > 0}>
                {
                    pointRecord.loading ? <div>Loading</div> :
                        <div>
                            <div>
                                {
                                    pointRecord.value && pointRecord.value.obj &&
                                    <ReactJson src={pointRecord.value.obj} />
                                }
                            </div>
                            {
                                pointRecord.value &&
                                pointRecord.value.records.map((item: any) => (
                                    <div key={item.id}>
                                        {
                                            item.type === "client" || item.type === "task" ?
                                                <ClientRecordItem record={item} />
                                                : item.type === "broker" ?
                                                    <BrokerRecordItem record={item} />
                                                    : <></>
                                        }
                                    </div>
                                ))
                            }
                        </div>
                }
            </Drawer> */}
        </div>
    )
}

export default GraphPage

const BrokerDetail = ({ broker, client, wid }: { broker: Broker, client: string, wid: number }) => {
    return (
        <div>
            <div className="font-bold text-2xl">{broker.name}(broker)</div>
            <TriggerForm object={broker} type="broker" client={client} wid={wid} />

            {
                broker.clients && typeof broker.clients == "object" ?
                    <div className="my-2 border-blue-500 border-dashed border-2 w-11/12 px-6 py-2">
                        <div>
                            <div className="flex items-center">
                                <MdOutlineSwitchAccessShortcut className="" />
                                <div className="font-bold ml-2">Run on</div>
                            </div>
                            {broker.clients.map((e: any) =>
                                <div key={e}>{e}</div>
                            )}
                        </div>
                    </div>

                    : <div></div>
            }
            <div className="my-2 border-blue-500 border-dashed border-2 w-11/12 px-6 py-2">
                {
                    broker.listens ?
                        <div>
                            <div className="flex items-center">
                                <MdOutlineSwitchAccessShortcut className="" />
                                <div className="font-bold ml-2">Listening</div>
                            </div>
                            {broker.listens.map((e: any) =>
                                <div key={e}>{e}</div>
                            )}
                        </div>
                        : <div></div>
                }
            </div>
            <div className="my-2 border-blue-500 border-dashed border-2 w-11/12 px-6 py-2">
                {
                    broker.flows ?
                        <div>
                            <div className="flex items-center">
                                <   TiFlowSwitch className="" />
                                <div className="font-bold ml-2">Flows</div>
                            </div>
                            {parseBrokerFlows(broker.flows).map((exp: any, idx: any) => (
                                <span key={idx}>
                                    {
                                        exp.map((e: any, edx: any) => (
                                            <span key={edx} className={`${e.className} whitespace-pre`}>{e.value}</span>
                                        ))
                                    }
                                </span>
                            ))}
                        </div>
                        : <div></div>
                }
            </div>
        </div>
    )
}

function getDefaultSchedule(object: any, type: string) {
    console.log({ object })
    if (type == "channel" || type == "broker") {
        return "Input"
    }

    if (object.schedule) {
        return object.schedule
    }

    if (object.wait) {
        return object.wait
    }

    return "Immediately"
}

const TaskDetail = ({ task, client, wid }: { task: Task, client: string, wid: number }) => {

    return (
        <div>
            <div className="font-bold text-2xl">{task.name}(task)</div>
            <TriggerForm object={task} type="task" client={client} wid={wid} />
            <div className="border-2 border-blue-500 px-2 w-11/12 my-4 py-2">
                <div className="my-2 flex ">
                    <div className="font-bold mr-2">Schedule</div>
                    {getDefaultSchedule(task, "task")}
                </div>
                <div className="flex items-center my-2">
                    <div className="font-bold mr-2">Executor</div>
                    <div>{task.executor}</div>
                </div>
                <div className="">
                    <div className="flex">
                        <div className="font-bold">Payload</div>
                    </div>
                    <div className="">
                        <ReactJson src={task && task.payload ? JSON.parse(task.payload) : {}} name={false} />
                    </div>
                </div>
            </div>
        </div>
    )
}


const ChannelDetail = ({ channel, wid }: { channel: Channel, wid: number }) => {
    return (
        <div>
            <div className="font-bold text-2xl">{channel.name}(channel)</div>
            <TriggerForm object={channel} type="channel" client="" wid={wid} />
        </div>
    )
}


const TriggerForm = ({ object, type, client, wid }: { object: any, type: string, client: string, wid: number }) => {
    const [input, setInput] = useState<any>(type == "task" || type == "channel" ? [{ name: "", value: {} }] : [])
    const [inputType, setInputType] = useState(1)
    const [inName, setInName] = useState("")
    const [openInput, setOpenInput] = useState(false)
    const [openSchedule, setOpenSchedule] = useState(false)
    const [schedule, setSchedule] = useState("")
    const [reload, setReload] = useState(0)
    const [err, setError] = useState("")
    const [openTrigger, setOpenTrigger] = useState<any>(null);
    const wfCmd = useRecoilValue(workflowMonitorAtom)
    const [triggers, setTriggers] = useState<Trigger[]>([])

    useAsync(async () => {
        console.log(wid, object, type)
        const ts = await services.workflows
            .getTrigger(wid, object.id, type)
            .catch(setError)
        if (Array.isArray(ts)) {
            for (const t of ts) {
                if (t.input) {
                    t.input = JSON.parse(t.input)
                }
            }
            setTriggers(ts)
            return ts
        }
        return []
    }, [reload])

    const saveTrigger = async () => {
        let value = ""
        if (type == "broker") {
            let v: any = {}
            for (const i of input) {
                v[i.name] = i.value
            }
            value = JSON.stringify(v)
        } else if (type == "task" || type == "channel") {
            let o = input[0]
            if (o) {
                value = JSON.stringify(o.value)
            } else {
                o = {}
            }
        }

        console.log(object)
        const trigger: Trigger = {
            id: 0,
            trigger_at: 0,
            schedule,
            input: value,
            object_id: object.id,
            type: type,
            name: object.name,
            client: client,
            workflow_id: wid,
            status: 0,
        }

        await services.workflows
            .saveTrigger(trigger)
            .catch(setError)

        setInput(type == "task" || type == "channel" ? [{ name: "", value: {} }] : [])
        setSchedule("")
        setReload(reload => reload + 1)
    }

    const deleteTrigger = async (id: any) => {
        await services.workflows
            .deleteTrigger(id)
            .catch(setError)
        setReload(reload => reload + 1)
    }

    useEffect(() => {
        if (wfCmd && wfCmd.cmd == TriggerStatusWorkflow) {
            const data = wfCmd.data
            setTriggers(trs => {
                const newTrs = [...trs]
                for (let i = 0; i < newTrs.length; ++i) {
                    if (newTrs[i].id == data.id) {
                        newTrs[i] = {
                            ...newTrs[i],
                            status: data.status
                        }
                        return newTrs
                    }
                }
                return trs
            })
        }
    }, [wfCmd])

    return (
        <div>
            <div className="flex-col items-center border-2 border-blue-500 border-dashed w-11/12 px-4 py-2">
                <div className="flex items-center ">
                    <div className="font-bold text-lg">Trigger</div>
                    <div className="cursor-pointer mx-2 hover:bg-yellow-500 w-10 h-8 bg-blue-500 flex items-center justify-center rounded-lg"
                        onClick={saveTrigger}>
                        <GrTrigger />
                    </div>
                </div>
                <div className="flex items-center my-2 ml-2">
                    <MdOutlineAccessTime className="mr-2" />
                    <div className="font-bold mr-2 ">Schedule</div>
                    <div>{schedule}</div>
                    <button className="mx-2" onClick={() => setOpenSchedule(true)}><IoMdAddCircleOutline /></button>
                </div>

                <div className="items-center my-2 border-green-500 border-dashed mx-2 border-2 px-2 py-2">
                    <div className="flex ">
                        <div className="font-bold mr-2">Input</div>
                        {
                            type == "broker" && <button className="mx-2" onClick={() => setOpenInput(true)}><IoMdAddCircleOutline /></button>
                        }
                    </div>
                    {
                        input.map((item: any, idx: number) => (
                            <div key={item.name} className="flex items-center">
                                <div>{item.name}</div>
                                <ReactJson
                                    src={item.value}
                                    name={false}
                                    enableClipboard={false}
                                    onAdd={(e) => {
                                        setInput((input: any) => {
                                            if (e.updated_src) {
                                                const newInput = [...input]
                                                newInput[idx].value = e.updated_src
                                                return newInput
                                            }
                                            return input
                                        })
                                    }}
                                    onDelete={(e) => {
                                        setInput((input: any) => {
                                            if (e.updated_src) {
                                                const newInput = [...input]
                                                newInput[idx].value = e.updated_src
                                                console.log("NEW INPU==", newInput)
                                                return newInput
                                            }
                                            return input
                                        })
                                    }}
                                    onEdit={(e) => {
                                        console.log(e)
                                        setInput((input: any) => {
                                            if (e.updated_src) {
                                                const newInput = [...input]
                                                newInput[idx].value = e.updated_src
                                                console.log("NEW INPU==", newInput)
                                                return newInput
                                            }
                                            return input
                                        })
                                    }}
                                />
                                {
                                    type == "broker" && (
                                        <FaDeleteLeft onClick={() => {
                                            const newI = input.filter((i: any) => i.name !== item.name)
                                            setInput([...newI])
                                        }}
                                            className="cursor-pointer"
                                        />
                                    )
                                }

                            </div>
                        ))
                    }
                </div>
                <div className="items-center my-2 border-green-500 border-dashed mx-2 border-2 px-2 py-2">
                    <div className="flex ">
                        <div className="font-bold mr-2">List trigger</div>
                        <div>
                            {
                                type == "broker" && <button className="mx-2" onClick={() => setOpenInput(true)}><IoMdAddCircleOutline /></button>
                            }
                        </div>
                    </div>

                    {
                        triggers.map(tri => (
                            <div key={tri.id}
                                className="flex cursor-pointer items-center ">
                                <div
                                    className="mr-2 w-24"
                                    onClick={() => { setOpenTrigger(tri) }}>
                                    {tri.schedule ? tri.schedule : "Immediately"}
                                </div>
                                <Link to={`/dashboard/timeline?trigger_id=${tri.id}`}>
                                    <div className="text-sm hover:text-gray-500">
                                        {
                                            tri.status == 0 ? <div>(Delivering...)</div>
                                                : tri.status == 1 ? <div>(Scheduled at {formatDate(tri.trigger_at)})</div>
                                                    : tri.status == 2 ? <div>(Finish)</div>
                                                        : <div>(Running)</div>
                                        }
                                    </div>
                                </Link>
                                <div className="ml-2">
                                    {
                                        tri.status != 2 && (
                                            <FaDeleteLeft onClick={() => { deleteTrigger(tri.id) }}
                                                className="cursor-pointer"
                                            />
                                        )
                                    }
                                </div>
                            </div>
                        ))
                    }
                </div>
            </div>


            <Modal
                open={openInput}
                onClose={() => { setOpenInput(false); setInName("") }}
                center
            >
                {
                    type == "broker" && (
                        <Radio.Group onChange={(e) => { setInputType(e.target.value) }} value={inputType}>
                            <Radio value={1}>Task</Radio>
                            <Radio value={2}>Channel</Radio>
                        </Radio.Group>
                    )
                }

                <div className="mx-2">{inName}{type == "broker" ? (inputType == 1 ? '_task' : '_channel') : ""}</div>
                <input placeholder="name"
                    autoFocus={true}
                    className="border-rounded mx-2 my-2 px-2 py-2"
                    onKeyDown={(e) => {
                        if (e.key == "Enter") {
                            setInput((input: any[]) => {
                                console.log("INPUT BEFORE", JSON.stringify(input))
                                for (const i of input) {
                                    if (i.name == inName) {
                                        Toast.error(`var ${inName} existed`)
                                        return input
                                    }
                                }
                                let value: any = {
                                    result: {
                                        output: "",
                                        success: false,
                                        finish: false,
                                    }
                                }
                                if (inputType == 2) {
                                    value = {
                                        content: {}
                                    }
                                }
                                const root = `${inName}${type == "broker" ? (inputType == 1 ? '_task' : '_channel') : ""}`
                                return [...input, { name: root, value }].filter(e => e.name && e.name != "_task" && e.name != "_channel")

                            })
                            setOpenInput(false); setInName("")
                        }
                    }}
                    onChange={(e) => { setInName(e.target.value) }} />

            </Modal>

            <Modal
                open={openSchedule}
                onClose={() => { setOpenSchedule(false) }}
                center
            >
                <div className="flex items-center mx-2 my-2" >
                    <button className="cursor-pointer bg-blue-500 text-white flex items-center px-2 rounded py-2">
                        <FaRegClock className="mr-2" onClick={() => { setOpenSchedule(false) }} />  Schedule
                    </button>
                    <div className="ml-2">
                        {schedule}
                    </div>
                </div>
                <div className="min-w-[600px] h-32">
                    <Cron
                        setValue={setSchedule}
                        value={schedule}
                    />
                </div>

            </Modal>

            <Modal
                open={openTrigger}
                onClose={() => { setOpenTrigger(null) }}
                center
            >
                {
                    openTrigger && (
                        <>
                            <div>{openTrigger.schedule}</div>
                            {
                                Array.isArray(openTrigger.input) ? (
                                    openTrigger.input.map((item: any) => (
                                        <div key={item.name}>
                                            <div>{item.name}</div>
                                            <ReactJson src={item.value} enableClipboard={false} />
                                        </div>
                                    ))
                                ) : <ReactJson src={openTrigger.input ? openTrigger.input : {}} enableClipboard={false} />
                            }
                        </>
                    )
                }
            </Modal>
        </div>
    )
}

const BrokerRecordItem = ({ record }: { record: any }) => {
    const [detail, setDetail] = useState(false)
    return (
        <>
            <div
                onClick={() => { setDetail(!detail) }}
                className={`${record.status ? "bg-green-800" : "bg-red-800"} flex cursor-pointer mx-2 my-2 px-1 py-1 rounded items-center`}>
                <div className="px-2 w-11/12">
                    <div>{record.name}</div>
                    <div>{formatDate(record.created_at)}</div>
                </div>
                {
                    detail ? <FaChevronDown /> : <FaChevronUp />
                }

            </div>
            {
                detail && <ReactJson src={(() => {
                    const value = {
                        input: record.input,
                        output: record.output,
                    }
                    return value
                })()} />
            }
        </>
    )
}

const ClientRecordItem = ({ record }: { record: any }) => {
    const [detail, setDetail] = useState(false)
    return (
        <>
            <div
                onClick={() => { setDetail(!detail) }}
                className={`${record.status ? "bg-green-800" : "bg-red-800"} flex cursor-pointer mx-2 my-2 px-1 py-1 rounded items-center`}>
                <div className="px-2 w-11/12">
                    <div>{record.name}</div>
                    <div>{formatDate(record.created_at)}</div>
                </div>
                {
                    detail ? <FaChevronDown /> : <FaChevronUp />
                }

            </div>
            {
                detail && <ReactJson src={(() => {
                    const value = {
                        started_at: formatDate(record.started_at),
                        finished_at: formatDate(record.finished_at),
                        task_name: record.task_name,
                        output: record.output,
                    }
                    return value
                })()} />
            }
        </>
    )
}

const ServiceMapOptions = {
    physics: {
        enabled: true,
        solver: 'forceAtlas2Based',
        forceAtlas2Based: {
            theta: 0.5,
            gravitationalConstant: -50,
            centralGravity: 0.01,
            springConstant: 0.08,
            springLength: 100,
            damping: 1.0,
            avoidOverlap: 0
        },
    },
    layout: {
        hierarchical: false,
        randomSeed: 1 // always on node 1
    },
    nodes: {
        shape: 'dot',
        chosen: true,
        font: {
            color: 'white',
            size: 8, // px
            face: 'arial',
            background: 'none',
            strokeWidth: 0, // px
            strokeColor: '#ffffff',
            align: 'center',
            multi: false
        },
        borderWidth: 1.5,
        borderWidthSelected: 2.5,
        labelHighlightBold: true,
        opacity: 1,
        shadow: true,
        scaling: {
            label: {
                min: 12,
                max: 24,
            },
        },
    },
    edges: {
        chosen: true,
        dashes: false,
        arrowStrikethrough: false,
        arrows: {
            to: {
                enabled: true,
            },
            middle: {
                enabled: false,
            },
            from: {
                enabled: false,
            }
        },
        smooth: {
            enabled: true,
            type: 'dynamic',
            roundness: 1.0
        },
        font: {
            color: '#343434',
            size: 8, // px
            face: 'arial',
            background: 'rgba(255,255,255,0.7)',
            strokeWidth: 2, // px
            strokeColor: '#ffffff',
            align: 'horizontal',
            multi: false
        },
        labelHighlightBold: true,
        selectionWidth: 1,
        shadow: true,
    },
    autoResize: true,
    interaction: {
        hover: true,
        multiselect: true,
    },
    groups: {},
};
