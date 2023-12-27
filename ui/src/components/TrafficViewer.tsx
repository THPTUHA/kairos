import React, { useCallback, useEffect, useRef, useState } from "react";
import makeStyles from '@mui/styles/makeStyles';
import variables from '../styles/variables.module.scss';
import { Entry } from "../models/entry";
import { useRecoilState, useRecoilValue, useSetRecoilState } from "recoil";
import { useNavigate, useSearchParams } from "react-router-dom";
import focusedItemAtom from "../recoil/focusedItem/atom";
import focusedStreamAtom from "../recoil/focusedStream/atom";
import focusedContextAtom from "../recoil/focusedContext/atom";
import queryAtom from "../recoil/query/atom";
import queryBuildAtom from "../recoil/queryBuild/atom";
import queryBackgroundColorAtom from "../recoil/queryBackgroundColor/atom";
import { BrokerPoint, BrokerPointColor, ChannelPoint, ChannelPointColor, ClientPoint, ClientPointColor, ColorYellow, KairosPointColor, SuccessCode, TaskPoint, TaskPointColor, ViewHistory, ViewRealtime } from "../conts";
import TrafficViewerStyles from "../styles/TrafficViewer.module.sass";
import { EntriesList } from "./entry/EntriesList";
import styles from '../styles/EntriesList.module.sass';
import { EntryDetailed } from "./entry/EntryDetail";
import { Filters } from "./Filters";
import Queryable from "./Queryable";
import { LuActivity } from "react-icons/lu";
import workflowMonitorAtom from "../recoil/workflowMonitor/atom";
import Modal from "react-responsive-modal";
import Tree, { CustomNodeElementProps } from 'react-d3-tree';
import { services } from "../services";
import { useAsync } from "react-use";
import { formatDate } from "../helper/date";
import { MessageFlow } from "../services/graphService";
import { Table } from "antd";
import { IoMdSend } from "react-icons/io";
import { MdInput, MdOutlineOutput } from "react-icons/md";

const useLayoutStyles = makeStyles(() => ({
  details: {
    flex: "0 0 50%",
    width: "45vw",
    padding: "12px 24px",
    borderRadius: 4,
    marginTop: 15,
    background: variables.headerBackgroundColor,
  },

  viewer: {
    display: "flex",
    overflowY: "auto",
    height: "calc(100% - 70px)",
    padding: 5,
    paddingBottom: 0,
    overflow: "auto",
  },
}));

interface TrafficViewerProps {
  entries: Entry[];
  actionButtons?: JSX.Element,
  viewType: number,
}

const DEFAULT_QUERY = "";

type TreeNode = {
  name: string,
  attributes: {
    type: number
  },
  id: number,
  type: number,
  part: string,
  owner_id: number,
  status: number,
  finish: boolean,
  owner_type: number,
  children: TreeNode[]
}

const getNameFromType = (type: number) => {
  if (type == ChannelPoint) {
    return "channel"
  } else if (type == TaskPoint) {
    return "task"
  } else if (type == BrokerPoint) {
    return "broker"
  } else if (type == ClientPoint) {
    return "client"
  }
  return "??"
}

function buildTree(mfs: MessageFlow[]) {
  const map = new Map<string, TreeNode>();
  const bidrect = new Map<string, MessageFlow>();
  let tree: TreeNode | {} = {};
  mfs = mfs.filter(item => {
    const key = `${item.parent}-${item.part}`
    const mf = bidrect.get(key)
    if (mf) {
      if (!item.flow) {
        bidrect.set(key, item)
        return true
      } else {
        mf.status = item.status
        mf.finish_part = item.finish_part
      }
      return false
    }
    bidrect.set(key, item)
    return true
  })

  mfs.forEach(item => {
    var c: any, p: any
    if (item.receiver_id == ClientPoint) {
      c = {
        name: item.task_name,
        id: item.task_id,
        type: TaskPoint,
        attributes: {
          type: TaskPoint,
          client: item.receiver_name
        },
        part: item.part,
        owner_id: item.receiver_id,
        owner_type: item.receiver_type,
      }
    } else {
      c = {
        name: item.receiver_name,
        id: item.receiver_id,
        type: item.receiver_type,
        attributes: {
          type: item.receiver_type,
        },
        part: item.part,
        owner_id: item.receiver_id,
        owner_type: item.receiver_type,
        children: []
      }
    }

    if (item.sender_id == ClientPoint) {
      p = {
        name: item.task_name,
        id: item.task_id,
        type: TaskPoint,
        attributes: {
          type: TaskPoint,
          client: item.sender_name,
          finish: item.finish_part,
        },
        status: item.status,
        part: item.part,
        owner_id: item.sender_id,
        owner_type: item.sender_type,
        children: []
      }
    } else {
      p = {
        part: item.part,
        name: item.sender_name,
        id: item.sender_id,
        type: item.sender_type,
        status: item.status,
        owner_id: item.sender_id,
        owner_type: item.sender_type,
        attributes: {
          type: item.sender_type,
          finish: item.finish_part,
        }
      }
    }
    map.set(item.part, {
      children: [c],
      ...p,
    });
  });

  mfs.forEach(item => {
    const parent = map.get(item.parent);
    if (parent) {
      const e = map.get(item.part)
      if (e) {
        let exist = false
        for (let i = 0; i < parent.children.length; i++) {
          const c = parent.children[i]
          if (c.owner_id == item.sender_id && c.owner_type == item.sender_type) {
            exist = true
            parent.children[i] = e
          }
        }
        if (!exist) {
          parent.children.push(e);
        }
      } else {

      }
    } else {
      const t = map.get(item.part)
      if (t) {
        tree = t
      }
    }
  });

  return tree;
}

export const TrafficViewer: React.FC<TrafficViewerProps> = ({
  entries,
  actionButtons,
  viewType
}) => {

  const classes = useLayoutStyles();
  const setFocusedItem = useSetRecoilState(focusedItemAtom);
  const setFocusedStream = useSetRecoilState(focusedStreamAtom);
  const setFocusedContext = useSetRecoilState(focusedContextAtom);
  const [query, setQuery] = useRecoilState(queryAtom);
  const setQueryBuild = useSetRecoilState(queryBuildAtom);
  const setQueryBackgroundColor = useSetRecoilState(queryBackgroundColorAtom);
  const [isSnappedToBottom, setIsSnappedToBottom] = useState(true);
  const [wsReadyState, setWsReadyState] = useState(0);
  const [searchParams] = useSearchParams();
  const [timeLineID, setTimeLine] = useState("");
  const entriesBuffer = useRef([] as Entry[]);

  const scrollableRef = useRef<any>(null);
  const ws = useRef<WebSocket>(null);
  const queryRef = useRef<string>("");
  queryRef.current = query;

  const navigate = useNavigate();

  useEffect(() => {
    const querySearchParam = searchParams.get("q");
    if (querySearchParam !== null) {
      setQueryBuild(querySearchParam);
      setQuery(querySearchParam);
    } else {
      setQueryBuild(DEFAULT_QUERY);
      setQuery(DEFAULT_QUERY);
      //   navigate({ pathname: location.pathname, search: `q=${encodeURIComponent(DEFAULT_QUERY)}` });
      setQueryBackgroundColor(ColorYellow);
    }

    let init = false;
    // if (!init) openWebSocket();
    return () => { init = true; }
  }, []);

  const closeWebSocket = useCallback((code: number) => {
    ws.current?.close(code);
  }, [ws]);

  const sendQueryWhenWsOpen = () => {
    setTimeout(() => {
      if (ws?.current?.readyState === WebSocket.OPEN) {
        ws.current.send(queryRef.current);
      } else {
        sendQueryWhenWsOpen();
      }
    }, 500);
  };

  const listEntry = useRef(null);

  const toggleConnection = useCallback(async () => {
    if (ws?.current?.readyState === WebSocket.OPEN) {
      closeWebSocket(4001);
    } else {
      // openWebSocket();
    }
    //@ts-ignore
    scrollableRef.current.jumpToBottom();
    setIsSnappedToBottom(true);
  }, [scrollableRef, setIsSnappedToBottom, closeWebSocket]);

  const reopenConnection = useCallback(async () => {
    closeWebSocket(1000);
    //@ts-ignore
    scrollableRef.current.jumpToBottom();
    setIsSnappedToBottom(true);
  }, [scrollableRef, setIsSnappedToBottom, closeWebSocket]);

  useEffect(() => {
    return () => {
      if (ws?.current?.readyState === WebSocket.OPEN) {
        ws.current.close();
      }
    };
  }, []);

  const onSnapBrokenEvent = () => {
    setIsSnappedToBottom(false);
  }

  if (ws.current && !ws.current.onmessage) {
    ws.current.onmessage = (e) => {
      if (!e?.data) return;
      const entry = JSON.parse(e.data);

      if (entriesBuffer.current.length === 0) {
        setFocusedItem(entry.id);
        setFocusedStream(entry.stream);
        setFocusedContext(entry.context);
      }

      entry.key = `${Date().valueOf()}-${entry.id}`;
      entriesBuffer.current.push(entry);
    }
  }

  return (
    <div className={`${TrafficViewerStyles.TrafficPage}`}>
      <div className={`${TrafficViewerStyles.TrafficPageHeader} bg-red-500`}>
        <div className={TrafficViewerStyles.TrafficPageStreamStatus}>
          {/* <img id="pause-icon"
            className={TrafficViewerStyles.playPauseIcon}
            style={{ visibility: wsReadyState === WebSocket.OPEN ? "visible" : "hidden" }}
            alt="pause"
            src={pauseIcon}
            onClick={toggleConnection} />
          <img id="play-icon"
            className={TrafficViewerStyles.playPauseIcon}
            style={{ position: "absolute", visibility: wsReadyState === WebSocket.OPEN ? "hidden" : "visible" }}
            alt="play"
            src={playIcon}
            onClick={toggleConnection} /> */}
          {/* <Switch defaultChecked onChange={changeSwitchView} /> */}
          <div className="flex">
            <Queryable
              query={`src.name == "kairos"`}
              displayIconOnMouseOver={true}
              flipped={true}
              iconStyle={{ marginRight: "10px" }}
            >
              <div className={`${KairosPointColor} mx-3`}>Kairos</div>
            </Queryable>
            <Queryable
              query={`src.name == "broker"`}
              displayIconOnMouseOver={true}
              flipped={true}
              iconStyle={{ marginRight: "10px" }}
            >
              <div className={`${BrokerPointColor}  mx-3`}>Broker</div>
            </Queryable>
            <Queryable
              query={`src.name == "client"`}
              displayIconOnMouseOver={true}
              flipped={true}
              iconStyle={{ marginRight: "10px" }}
            >
              <div className={`${ClientPointColor}  mx-3`}>Client</div>
            </Queryable>
            <Queryable
              query={`src.name == "channel"`}
              displayIconOnMouseOver={true}
              flipped={true}
              iconStyle={{ marginRight: "10px" }}
            >
              <div className={`${ChannelPointColor} mx-3`}>Channel</div>
            </Queryable>
            <Queryable
              query={`src.name == "task"`}
              displayIconOnMouseOver={true}
              flipped={true}
              iconStyle={{ marginRight: "10px" }}
            >
              <div className={`${TaskPointColor} mx-3`}>Task</div>
            </Queryable>
          </div>
        </div>
        {actionButtons}
      </div>

      {<div className={TrafficViewerStyles.TrafficPageContainer}>
        <div className={TrafficViewerStyles.TrafficPageListContainer}>
          <Filters
            entries={entries}
            reopenConnection={reopenConnection}
            onQueryChange={(q) => { setQueryBuild(q?.trim()); }}
          />
          {
            (viewType == ViewRealtime || viewType == ViewHistory) ? (
              <div className={styles.container}>
                <EntriesList
                  entries={entries}
                  listEntryREF={listEntry}
                  onSnapBrokenEvent={onSnapBrokenEvent}
                  isSnappedToBottom={isSnappedToBottom}
                  setIsSnappedToBottom={setIsSnappedToBottom}
                  scrollableRef={scrollableRef}
                />
              </div>
            ) : (
              <TimeLineView setTimeLine={setTimeLine} />
            )
          }
        </div>
        <div className={classes.details} id="rightSideContainer">
          {
            (viewType == ViewRealtime || viewType == ViewHistory) ? (
              <EntryDetailed />
            ) : (
              <TimeLineDetail groupID={timeLineID} />
            )
          }
        </div>
      </div>}
    </div>
  );
};

const EntryTime = ({ mf }: { mf: MessageFlow }) => {
  return (
    <div className={`flex justify-between h-12 bg-blue-800 items-center px-2 mx-2 my-2 rounded`}>
      <div className="">
        <LuActivity />
      </div>
      <div>{mf.workflow_name}</div>
      <div>{mf.sender_name}</div>
      <div>{formatDate(mf.created_at)}</div>
    </div>
  )
}

const TimeLineView = ({ setTimeLine }: { setTimeLine: any }) => {
  const [error, setError] = useState<Error>();

  const mfs = useAsync(async () => {
    const mfs = await services.graphs
      .getTimeLine()
      .catch(setError)
    if (Array.isArray(mfs)) {
      return mfs
    }
    return []
  }, [])

  return (
    <div className="">
      <div className="">
        {
          mfs.loading ? <div>Loading</div>
            : <div>
              {
                mfs.value?.map(e => (
                  <div key={e.id} onClick={() => { setTimeLine(e.group) }}
                    className="cursor-pointer">
                    <EntryTime mf={e} />
                  </div>
                ))
              }
            </div>
        }
      </div>
    </div>
  )
}

interface Edge {
  id: string;
  from: string;
  to: string;
  value: number;
  count: number;
  cumulative: number;
  label: string;
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

type NodeSelected = {
  inputs: MessageFlow[],
  outputs: MessageFlow[],
  type: number,
  name: string,
}

const TimeLineDetail = ({ groupID }: { groupID: string }) => {
  const modalRef = useRef(null);
  const wfCmd = useRecoilValue(workflowMonitorAtom)
  const [nodeSelected, setNodeSelected] = useState<NodeSelected | null>();
  const [error, setError] = useState<Error>();


  const [treeData, setTreeData] = useState<any>({});

  const onNodeClick = (nodeData: any) => {
    // Handle node click event, e.g., display a popup
    alert(nodeData.attributes.popupContent);
  };

  const graphS = useAsync(async () => {
    if (groupID) {
      const mfs = await services.graphs
        .getGroupID(groupID)
        .catch(setError)

      const data: any = {}
      if (Array.isArray(mfs)) {
        const gs = mfs.filter(e =>
          e.sender_type == ChannelPoint || e.receiver_type == ChannelPoint || e.task_id
        )
        if (mfs.length) {
          const tr = buildTree(mfs)
          console.log("DATA", tr)
          setTreeData(tr)
        }
        return gs
      }
    }
    return []
  }, [groupID])



  useEffect(() => {
    // console.log("???", wfCmd)
  }, [wfCmd])

  useEffect(() => {
    if (groupID) {
      // todo add time line
    }
  }, [groupID])

  async function handleClick(hierarchyPointNode: any) {
    const parts = []
    console.log("hierarchyPointNode", hierarchyPointNode)
    if (hierarchyPointNode.parent) {
      if (hierarchyPointNode.parent.data.part) {
        parts.push(hierarchyPointNode.parent.data.part)
      }
    }
    // if(hierarchyPointNode.children){
    //   for(const c of hierarchyPointNode.children){
    //     if(c.data.part){
    //       parts.push(c.data.part)
    //     }
    //   }
    // }
    console.log("PART---", parts)
    const paths = await services.graphs
      .getParts(parts, parts)
      .catch(setError)

    if (paths) {
      setNodeSelected({
        type: hierarchyPointNode.data.type,
        inputs: paths.inputs.reverse(),
        outputs: paths.outputs.reverse(),
        name: hierarchyPointNode.data.name,
      })
    }
    console.log("PARTS---", paths)
  }

  return (
    <>
      <Modal
        open={nodeSelected != null}
        onClose={() => { setNodeSelected(null) }}
        center
      >
        <div className="min-w-[400px]" >
          {
            nodeSelected && <div>
              <div>{getNameFromType(nodeSelected.type)} : { }</div>
              <div>
                {
                  nodeSelected.type === BrokerPoint ?
                    <BorkerDetail
                      inputs={nodeSelected.inputs}
                      outputs={nodeSelected.outputs}
                    /> :
                    nodeSelected.type === TaskPoint ?
                      <TaskDetail
                        inputs={nodeSelected.inputs}
                        outputs={nodeSelected.outputs}
                      /> :
                      <></>
                }
              </div>
            </div>
          }
        </div>
      </Modal>
      <Tree
        data={treeData}
        orientation="vertical"
        renderCustomNodeElement={({ nodeDatum, toggleNode, hierarchyPointNode }) => {
          return (
            <>
              {
                nodeDatum.attributes?.type == ChannelPoint ?
                  <g >
                    {
                      <rect width="40" height="40" x="-20" fill="orange" onClick={() => { handleClick(hierarchyPointNode) }} />
                    }
                    <text fill="black" strokeWidth="1" x="30" y="10">
                      {nodeDatum.name}
                    </text>
                    {nodeDatum.attributes?.point && (
                      <text fill="black" x="20" dy="20" strokeWidth="1">
                        Point: {nodeDatum.attributes?.point}
                        {nodeDatum.attributes?.client ? "Client: " + nodeDatum.attributes?.client : ""}
                      </text>
                    )}
                  </g>
                  : nodeDatum.attributes?.type == TaskPoint ? <g className={`${nodeDatum.attributes.finish ? "" : "active"}`}>
                    <circle r="20" onClick={() => { handleClick(hierarchyPointNode) }} />
                    <text fill="black" strokeWidth="1" x="30" y="10">
                      {nodeDatum.name}
                    </text>
                    {nodeDatum.attributes?.point && (
                      <text fill="black" x="20" dy="20" strokeWidth="1">
                        Point: {nodeDatum.attributes?.point}
                        {nodeDatum.attributes?.client ? "Client: " + nodeDatum.attributes?.client : ""}
                      </text>
                    )}
                  </g>
                    : nodeDatum.attributes?.type == BrokerPoint ? <g className={`${nodeDatum.attributes.finish ? "" : "active"}`}>
                      <ellipse rx="25" ry="15" fill="green" onClick={() => { handleClick(hierarchyPointNode) }} />
                      <text fill="black" strokeWidth="1" x="30" y="10">
                        {nodeDatum.name}
                      </text>
                      {nodeDatum.attributes?.point && (
                        <text fill="black" x="20" dy="20" strokeWidth="1">
                          Point: {nodeDatum.attributes?.point}
                          {nodeDatum.attributes?.client ? "Client: " + nodeDatum.attributes?.client : ""}
                        </text>
                      )}
                    </g> : ""
              }
            </>

          )
        }}
      />
    </>
  )
}

type ItemChart = {
  request: MessageFlow,
  response?: MessageFlow,
  input: MessageFlow
}

const BorkerDetail = ({ outputs, inputs }: { outputs: MessageFlow[], inputs: MessageFlow[] }) => {
  var itemCharts: ItemChart[] = []
  for (let i = 0; i < outputs.length; i++) {
    if (!outputs[i].flow) {
      let request = outputs[i]
      let response, input: any
      for (let j = 0; j < outputs.length; j++) {
        if (outputs[i].part == outputs[j].part && outputs[i].parent == outputs[j].parent && outputs[j].flow) {
          response = outputs[j]
          break
        }
      }

      for (const e of inputs) {
        if (e.part == request.parent && e.broker_group == request.broker_group) {
          input = e
          break
        }
      }
      itemCharts.push({
        request: request,
        response: response,
        input: input
      })
    }
  }

  console.log(",,,,", itemCharts)
  return (
    <div>
      {
        itemCharts.map((e, idx) => (
          <div key={idx}>
            <div><MdInput />Input</div>
            <div>
              <div>Sender</div>
              <div>{e.input.sender_name}{`(${getNameFromType(e.input.sender_type)})`}</div>
              <div>Value: {e.input.message}</div>
            </div>
            <div><MdOutlineOutput />Ouput:</div>
            <div>
              <div>
                <div>Receiver</div>
                <div>{e.request.receiver_name}{`(${getNameFromType(e.request.receiver_type)})`}</div>
                <div>Value: {e.request.message}</div>
              </div>
            </div>
            <div>Response</div>
            <div>
              {
                e.response ? <div>
                  <div>{e.response.receiver_name}{`(${getNameFromType(e.response.receiver_type)})`}</div>
                  <div>Value: {e.response.message}</div>
                </div>
                  : <div>No response</div>
              }
            </div>
          </div>
        ))
      }
    </div>
  )
}

const TaskDetail = ({ outputs, inputs }: { outputs: MessageFlow[], inputs: MessageFlow[] }) => {
  const input = inputs.filter(e => !e.flow)[0]
  const value = JSON.parse(input.message).input;
  const outs = outputs.map(e => {
    e.outobject = JSON.parse(e.message)
    return e
  })
  return (
    <div>
      <div>Input</div>
      <div>Sender</div>
      <div>{input.sender_name}{`(${getNameFromType(input.sender_type)})`}</div>
      <div>Value: {value}</div>

      <div>Output</div>
      <div>
        {
          outs.map(e => (
            <div key={e.id} className="flex justify-between">
              <div>{e.receive_at}</div>
              <div>{e.outobject.success ? "success" : "fault"}</div>
              <div>{e.outobject.output}</div>
            </div>

          ))
        }
      </div>
    </div>
  )
}
// Component để hiển thị label với màu
const CustomLabel = ({ nodeData }: { nodeData: any }) => (
  <g>
    <circle r="10" fill={nodeData.attributes.color} stroke="black" strokeWidth="2" />
    <text x="15" y="5">{nodeData.name}</text>
  </g>
);

