import { useEffect, useRef, useState } from "react";
import ForceGraph from "../libs/ForceGraph"
import { useAsync } from "react-use";
import { services } from "../services";
import { Checkbox, Drawer, Menu, Radio, RadioChangeEvent, Tabs } from "antd";
import { CheckboxValueType } from "antd/es/checkbox/Group";
import { BrokerPoint, ChannelPoint, ClientPoint, ColorGreen, ColorRed, TaskPoint } from "../conts";
import { Utils } from "../helper/utils";
import { formatDate } from "../helper/date";
import { FaChevronDown, FaChevronUp } from "react-icons/fa6";
import ReactJson from "react-json-view";

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
    const [brokers, setBrokers] = useState<any[]>([])

    const [graphType, setGraphType] = useState(1)
    const [wfIDs, setWfIDs] = useState<string[]>([])
    const [error, setError] = useState<Error>();


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
        console.log(node)
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
            console.log({result})
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

    console.log("pointRecord", pointRecord.value)
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

        const graphData = await services.graphs
            .data({
                workflow_ids: wfIDs.map(id => parseInt(id))
            })
            .catch(setError)
        const tasks = []
        const brokers = []
        const clients = []
        if (Array.isArray(graph)) {
            var nodes: Node[] = []
            var edges: Edge[] = []
            for (const g of graph) {
                if (g.clients) {
                    for (const client of g.clients) {
                        clients.push(client)
                        let exist = false
                        for (const n of nodes) {
                            if (n.id === `${client.id}-client`) {
                                exist = true
                            }
                        }
                        if (!exist) {
                            nodes.push({
                                id: `${client.id}-client`, label: client.name, group: g.id, value: 10, color: "blue"
                            })
                        }
                    }
                }
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
                    }
                }

                if (g.tasks) {
                    const keys = Object.keys(g.tasks)
                    for (const key of keys) {
                        const task = g.tasks[key]
                        tasks.push(task)
                        nodes.push({
                            id: `${task.id}-task`, label: `${task.name}-${g.name}`, group: g.id, value: 10, color: "pink"
                        })
                        if (task.clients) {
                            for (const c of task.clients) {
                                if (g.clients) {
                                    for (const client of g.clients) {
                                        if (client.name === c) {
                                            edges.push({
                                                id: `${task.id}-task#${client.id}-client`, from: `${task.id}-task`, to: `${client.id}-client`, value: 10, count: 10, cumulative: 123, label: "run", filter: "", proto: ""
                                            })
                                        }
                                    }
                                }
                            }
                        }
                    }
                }

                if (g.brokers) {
                    const keys = Object.keys(g.brokers)
                    for (const key of keys) {
                        const broker = g.brokers[key]
                        brokers.push(broker)
                        nodes.push({
                            id: `${broker.id}-broker`, label: `${broker.name}-${g.name}`, group: g.id, value: 10, color: "#FFA500"
                        })
                        for (const l of broker.listens) {
                            for (const client of g.clients) {
                                if (l == `${client.name}_client`) {
                                    edges.push({
                                        id: `${client.id}-client#${broker.id}-broker`, from: `${client.id}-client`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "listen", filter: "", proto: ""
                                    })
                                }
                            }

                            if (g.tasks) {
                                const keys = Object.keys(g.tasks)
                                for (const key of keys) {
                                    const task = g.tasks[key]
                                    if (l == `${task.name}_task`) {
                                        edges.push({
                                            id: `${task.id}-task#${broker.id}-broker`, from: `${task.id}-task`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "listen", filter: "", proto: ""
                                        })
                                    }
                                }
                            }

                            for (const channel of g.channels) {
                                if (l == `${channel.name}_channel`) {
                                    edges.push({
                                        id: `${channel.id}-channel#${broker.id}-broker`, from: `${channel.id}-channel`, to: `${broker.id}-broker`, value: 10, count: 10, cumulative: 123, label: "listen", filter: "", proto: ""
                                    })
                                }
                            }
                        }
                    }
                }
            }
            console.log({ edges, nodes })
            const edgesNew: Edge[] = []
            for (const data of graphData) {
                var send_type = getTypeUser(data.sender_type)
                var receiver_type = getTypeUser(data.receiver_type)
                var value = 10
                var label = "listen"
                switch (graphType) {
                    case 2:
                        value = data.count
                        label = `${value}`
                        break;
                    case 3:
                        value = Math.round(data.elapsed_time_avg)
                        label = `${value} ms`
                        break;
                    case 4:
                        value = Math.round(data.throughput)
                        label = `${Utils.humanReadableBytes(value)}`
                        break;
                }
                edgesNew.push({
                    id: `${data.sender_id}-${send_type}#${data.receiver_id}-${receiver_type}@${data.workflow_id}`, from: `${data.sender_id}-${send_type}`, to: `${data.receiver_id}-${receiver_type}`, value: value, count: 10, cumulative: 123, label: label, filter: "", proto: ""
                })
            }
            setTasks(tasks)
            setClients(clients)
            setBrokers(brokers)
            setGraphData({
                nodes: nodes,
                edges: graphType === 1 ? edges : edgesNew,
            })
            return graph
        }

        return []
    }, [wfIDs, graphType])

    function onSelectWorkflow(selects: CheckboxValueType[]) {
        setWfIDs(selects.map(item => item.toString()))
    }

    function onChangeTypeGraph(e: RadioChangeEvent) {
        setGraphType(e.target.value)
    }


    return (
        <div className="relative">
            <div className="w-1/3  absolute">
                <div>Workflow</div>
                {
                    wfs.loading ? <div>Loading</div> :
                        <Checkbox.Group options={wfs.value && wfs.value.map(item => ({
                            label: item.name, value: item.id
                        }))} onChange={onSelectWorkflow} />
                }
                <div>Type</div>
                <Radio.Group onChange={onChangeTypeGraph} value={graphType}>
                    <Radio value={1}>Connect</Radio>
                    <Radio value={2}>Deliver count</Radio>
                    <Radio value={3}>Avg Elapsed Time</Radio>
                    <Radio value={4}>Throughput</Radio>
                </Radio.Group>
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
            <Drawer title="Record" placement="right" onClose={onCloseDetail} open={selectedNodes.length > 0 || selectedEdges.length > 0}>
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
                                            :item.type === "broker" ?
                                            <BrokerRecordItem record={item}/>
                                            :<></>
                                       }
                                    </div>
                                ))
                            }
                        </div>
                }
            </Drawer>
        </div>
    )
}

export default GraphPage

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
            color: '#343434',
            size: 14, // px
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
            size: 12, // px
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
