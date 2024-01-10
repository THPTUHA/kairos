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
import { BrokerPoint, BrokerPointColor, ChannelPoint, ChannelPointColor, ClientPoint, ClientPointColor, ColorYellow, KairosPoint, KairosPointColor, LogMessageFlow, ObjectStatusWorkflow, SuccessCode, TaskPoint, TaskPointColor, ViewHistory, ViewRealtime } from "../conts";
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
import { useAsync, useLocation } from "react-use";
import { formatDate } from "../helper/date";
import { MessageFlow } from "../services/graphService";
import { Table } from "antd";
import { IoMdSend } from "react-icons/io";
import { MdInput, MdOutlineOutput } from "react-icons/md";
import ReactJson from "react-json-view";
import { FaArrowRightLong } from "react-icons/fa6";
import { FaLongArrowAltDown } from "react-icons/fa";
import { Task } from "kairos-js";
import { Broker } from "../models/broker";
import { parseBrokerFlows } from "../helper/base";
import { TiFlowSwitch } from "react-icons/ti";
import { RiAddLine } from "react-icons/ri";
import { Client } from "../models/client";
import { CgTrack } from "react-icons/cg";

const useLayoutStyles = makeStyles(() => ({
  details: {
    flex: "0 0 50%",
    width: "45vw",
    padding: "12px 24px",
    borderRadius: 4,
    marginTop: 15,
    background: variables.headerBackgroundColor,
  },

  timelineDetails: {
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
  setOffset: any,
}

const DEFAULT_QUERY = "";

type TreeNode = {
  name: string,
  attributes: {
    type: number
    start_input: string
  },
  id: number,
  type: number,
  part: string,
  parent: string,
  owner_id: number,
  status: number,
  finish: boolean,
  owner_type: number,
  start_input: string,
  children: TreeNode[]
}

const Running = 1
const Finish = 2
const Fault = 3

const getNameFromType = (type: number) => {
  if (type == ChannelPoint) {
    return "Channel"
  } else if (type == TaskPoint) {
    return "Task"
  } else if (type == BrokerPoint) {
    return "Broker"
  } else if (type == ClientPoint) {
    return "Client"
  }
  return "??"
}

function mergeTree(a: TreeNode, b: TreeNode) {
  // console.log(a.name, b.name, JSON.stringify(a.children), JSON.stringify(b.children))
  if (a.owner_id == b.owner_id && a.owner_type == b.owner_type) {
    a.finish = b.finish
    a.attributes = b.attributes
    a.status = b.status
    a.part = b.part
    const newChild = []
    if (Array.isArray(a.children)) {
      for (const c1 of a.children) {
        let exist = false
        if (Array.isArray(b.children)) {
          for (const c2 of b.children) {
            if (c1.owner_id == c2.owner_id && c1.owner_type == c2.owner_type) {
              exist = true
              mergeTree(c1, c2)
            }
          }
        }
        if (!exist) {
          newChild.push(c1)
        }
      }
    }

    if (Array.isArray(b.children)) {
      for (const c1 of b.children) {
        let exist = false
        if (Array.isArray(a.children)) {
          for (const c2 of a.children) {
            if (c1.owner_id == c2.owner_id && c1.owner_type == c2.owner_type) {
              exist = true
            }
          }
        }
        if (!exist) {
          newChild.push(c1)
        }
      }
    }
    // console.log("NEW CHILD", newChild)
    a.children = newChild
    b.children = newChild
  }
}

function getClient(cl: string, clients: Client[]) {
  for (const c of clients) {
    if (c.name == cl) {
      console.log(cl, clients, "?/")
      return c
    }
  }
  return {} as any
}

function buildTree(mfs: MessageFlow[], clients: Client[], tasks: Task[], brokers: Broker[]) {
  const map = new Map<string, TreeNode>();
  const bidrect = new Map<string, MessageFlow>();
  let tree: TreeNode | {} = {};
  let selfroot: any = {}
  let brokerTrigger: any = {}
  let parent: any = {}
  let root_cnt = 0
  mfs = mfs.filter(item => {
    // if (item.flow === 3) {
    //   return false 
    // }
    if (!item.part && !item.start_input) {
      return false
    }
    if (item.parent == item.part) {
      selfroot = item
      root_cnt++
      return false
    }
    if (!item.parent && item.part != parent.part) {
      console.log("PARENT---", item)
      parent = item
      root_cnt++
    }
    const key = `${item.parent}-${item.part}`
    const mf = bidrect.get(key)
    if (mf) {
      // từ broker luồng gửi
      if (!item.flow || item.sender_type == BrokerPoint) {
        bidrect.set(key, item)
        return true
      } else if (item.flow == 1 || item.flow == 3) {
        // luồng nhận
        mf.status = item.status
        mf.finish_part = item.finish_part
        if (item.message) {
          try {
            mf.response = JSON.parse(item.message)
          } catch (error) {
            console.error(error)
          }
        }
      }
      return false
    }
    if (item.flow == 1 || item.flow == 3) {
      if (item.message) {
        try {
          item.response = JSON.parse(item.message)
        } catch (error) {
          console.error(error)
        }
      }
    }
    bidrect.set(key, item)
    return true
  })

  console.log("MFS", mfs)
  // if (root_cnt == 2 && selfroot.part != parent.part) {
  //   console.error("invalid root", root_cnt, selfroot.part, parent.part)
  //   return {}

  // } else if (root_cnt != 1 && root_cnt != 2) {
  //   console.error("invalid root", root_cnt)
  //   return {}
  // }
  console.log("selfroot", selfroot)

  if (selfroot && !mfs.length) {
    let success, finish
    if (selfroot.message) {
      try {
        const mes = JSON.parse(selfroot.message)
        success = mes.success
        finish = mes.finished_at > 0
      } catch (error) {
        console.error(error)
      }
    }
    tree = {
      name: selfroot.task_name,
      id: selfroot.task_id,
      type: TaskPoint,
      attributes: {
        id: selfroot.task_id,
        type: TaskPoint,
        client: selfroot.sender_name,
        success: success,
        finish: finish,
        status: selfroot.status,
        active: getClient(selfroot.sender_name, clients).status,
        start_input: selfroot.start_input,
      },
      status: selfroot.status,
      part: selfroot.part,
      owner_id: selfroot.sender_id,
      owner_type: selfroot.sender_type,
      children: []
    }
    return tree
  }


  try {
    mfs.forEach(item => {
      var c: any, p: any
      if (item.receiver_type == ClientPoint && item.message) {
        const msg = JSON.parse(item.message)
        let success = false
        if (msg.success == undefined || msg.success) {
          success = true
        }
        c = {
          name: item.task_name,
          id: item.task_id,
          type: TaskPoint,
          attributes: {
            id: item.task_id,
            type: TaskPoint,
            client: item.receiver_name,
            // success: success,
            // finish: item.finish_part,
            status: item.status,
            active: getClient(item.receiver_name, clients).status,
            tracking: item.tracking,
            start_input: item.start_input,
            message: item.message,
          },
          status: item.status,
          part: item.part,
          parent: item.part,
          owner_id: item.receiver_id,
          owner_type: item.receiver_type,
        }
        if (item.response) {
          c.attributes.success = item.response.success
          c.attributes.finish = item.response.finished_at > 0
        }
      } else if (item.receiver_type != KairosPoint) {
        c = {
          name: item.receiver_name,
          id: item.receiver_id,
          type: item.receiver_type,
          attributes: {
            id: selfroot.receiver_id,
            type: item.receiver_type,
            status: item.status,
            tracking: item.tracking,
            start_input: item.start_input,
            message: item.message,
          },
          status: item.status,
          part: item.part,
          parent: item.part,
          owner_id: item.receiver_id,
          owner_type: item.receiver_type,
          children: []
        }
      }

      if (item.sender_type == ClientPoint) {
        if (!item.message) {
          console.error("empty message", item)
          if (item.task_id == 0 && item.receiver_type == BrokerPoint) {
            brokerTrigger = item
            // p = {
            //   name: item.receiver_name,
            //   id: item.receiver_id,
            //   type: BrokerPoint,
            //   attributes: {
            //     id: item.task_id,
            //     type: BrokerPoint,
            //     client: item.sender_name,
            //     finish: item.finish_part,
            //     status: item.status,
            //     tracking: item.tracking,
            //     start_input: item.start_input,
            //     message: item.message,
            //   },
            //   status: item.status,
            //   part: item.part,
            //   parent: item.parent,
            //   owner_id: item.sender_id,
            //   owner_type: item.sender_type,
            //   children: []
            // }
          }
        } else {
          const msg = JSON.parse(item.message)
          let success = false
          if (msg.success == undefined || msg.success) {
            success = true
          }
          p = {
            name: item.task_name,
            id: item.task_id,
            type: TaskPoint,
            attributes: {
              id: item.task_id,
              type: TaskPoint,
              client: item.sender_name,
              finish: item.finish_part,
              success: success,
              status: item.status,
              tracking: item.tracking,
              start_input: item.start_input,
              message: item.message,
            },
            status: item.status,
            part: item.part,
            parent: item.parent,
            owner_id: item.sender_id,
            owner_type: item.sender_type,
            children: []
          }
        }

      } else if (item.sender_type == KairosPoint) {
        p = {
          part: item.part,
          name: item.receiver_name,
          id: item.receiver_id,
          type: item.receiver_type,
          status: item.status,
          owner_id: item.receiver_id,
          parent: item.parent,
          owner_type: item.receiver_type,
          attributes: {
            id: item.receiver_id,
            type: item.receiver_type,
            finish: item.finish_part,
            status: item.status,
            tracking: item.tracking,
            start_input: item.start_input,
            message: item.message,
          }
        }
      } else {
        p = {
          part: item.part,
          name: item.sender_name,
          id: item.sender_id,
          type: item.sender_type,
          status: item.status,
          owner_id: item.sender_id,
          parent: item.parent,
          owner_type: item.sender_type,
          attributes: {
            id: item.sender_id,
            type: item.sender_type,
            finish: item.finish_part,
            status: item.status,
            tracking: item.tracking,
            start_input: item.start_input,
            message: item.message,
          }
        }
      }

      const children = []
      if (c && p) {
        if (!(p.id == c.id && p.type == c.type)) {
          children.push(c)
        }
      }
      if (p) {
        map.set(item.part, {
          ...p,
          children: children,
        });
      }

    });

  } catch (error) {
    console.error(error)
  }

  map.forEach((value, key) => {
    const cp = JSON.stringify(value)
    console.log(key, JSON.parse(cp))
  })

  const parents = []
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
            // parent.children[i] = e
            mergeTree(parent.children[i], e)
          }
        }
        if (!exist) {
          // console.log(JSON.stringify(e), )
          // console.log(JSON.stringify(parent), parent.children)
          if (parent.owner_id == e.owner_id && parent.owner_type == e.owner_type && parent.name == e.name) {
            parents.push(e.part)
          } else {

            parent.children.push(e);
            // console.log("RUN HERE", JSON.stringify(parent.children))
          }

        }
      } else {

      }
    }
  });

  if (selfroot.id) {
    parents.push(selfroot.part)
  } else if (parent.id) {
    parents.push(parent.part)
  }

  let p = map.get(parents[0])
  for (let i = 1; i < parents.length; i++) {
    if (p) {
      const c = map.get(parents[i])
      if (c) {
        p.children.push(...c.children)
      }
    }
  }

  console.log("FUCK PP", p)
  if (!p && selfroot.id) {
    console.log("RUN HERE????")
    const parents: any[] = []
    map.forEach((v, k) => {
      if (v.parent == selfroot.part) {
        parents.push(v)
      }
    })
    p = parents[0]
    if (p) {
      for (let i = 1; i < parents.length; ++i) {
        mergeTree(p, parents[i])
      }
    }
  }

  if (!p && brokerTrigger.id) {
    const pn: any[] = []
    map.forEach((v, k) => {
      if (v.parent == brokerTrigger.part) {
        pn.push(v)
        v.start_input = brokerTrigger.start_input
        v.attributes.start_input = brokerTrigger.start_input
      }
    })
    console.log({ pn })
    p = pn[0]
    if (p) {
      for (let i = 1; i < pn.length; ++i) {
        mergeTree(p, pn[i])
      }
    }
  }
  // console.log("PARENT", p, parents)
  return p;
}

type TimeLineSelected = {
  group: string,
  workflow_id: number,
}

export const TrafficViewer: React.FC<TrafficViewerProps> = ({
  entries,
  actionButtons,
  viewType,
  setOffset
}) => {

  const classes = useLayoutStyles();
  const setFocusedItem = useSetRecoilState(focusedItemAtom);
  const setFocusedStream = useSetRecoilState(focusedStreamAtom);
  const setFocusedContext = useSetRecoilState(focusedContextAtom);
  const [query, setQuery] = useRecoilState(queryAtom);
  const setQueryBuild = useSetRecoilState(queryBuildAtom);
  const setQueryBackgroundColor = useSetRecoilState(queryBackgroundColorAtom);
  const [isSnappedToBottom, setIsSnappedToBottom] = useState(true);
  const [searchParams] = useSearchParams();
  const [timeline, setTimeline] = useState<TimeLineSelected | null>(null);
  const [error, setError] = useState(null)
  const entriesBuffer = useRef([] as Entry[]);

  const scrollableRef = useRef<any>(null);
  const ws = useRef<WebSocket>(null);
  const queryRef = useRef<string>("");
  queryRef.current = query;

  const location = useLocation();
  useAsync(async () => {
    if (location.search) {
      const params = new URLSearchParams(location.search)
      const triggerID = params.get("trigger_id")
      console.log({triggerID})
      if (triggerID) {
        const mfs = await services.graphs
          .getTimeLine(triggerID)
          .catch(setError)
        if (Array.isArray(mfs)) {
          if(mfs.length > 1){
            console.error("More than 1 trigger")
          }else{
            const mf =mfs[0]
            setTimeline({group: mf.group, workflow_id: mf.workflow_id})
          }
          return mfs
        }
        return []
      }
    }
  }, [location])

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
                  setOffset={setOffset}
                />
              </div>
            ) : (
              <><TimeLineView setTimeline={setTimeline} timeline={timeline} /></>
            )
          }
        </div>
        {
          (viewType == ViewRealtime || viewType == ViewHistory) ? (
            <div className={classes.details} id="rightSideContainer">
              <EntryDetailed />
            </div>

          ) : (
            <div className={classes.timelineDetails}>
              {timeline && <TimeLineDetail timeline={timeline} />}
            </div>
          )
        }
      </div>}
    </div>
  );
};

const EntryTime = ({ mf, setTimeline }: { mf: MessageFlow, setTimeline: any }) => {
  return (
    <div
      onClick={() => { setTimeline({ group: mf.group, workflow_id: mf.workflow_id }) }}
      className={`flex justify-between h-12 items-center`}>
      <div
        className="flex items-center">
        <LuActivity />
        <Queryable
          query={`src.workflow == "${mf.workflow_name}"`}
          displayIconOnMouseOver={true}
          flipped={true}
          iconStyle={{ marginRight: "10px" }}
        >
          <div>{mf.workflow_name}</div>
        </Queryable>
      </div>

      <div>{mf.sender_name}</div>
      <div>{formatDate(mf.created_at)}</div>
    </div>
  )
}

const TimeLineView = ({ setTimeline, timeline }: { setTimeline: any, timeline: TimeLineSelected | null }) => {
  const [error, setError] = useState<Error>();
  const wfCmd = useRecoilValue(workflowMonitorAtom)
  const [reload, setReload] = useState(1)

  const mfs = useAsync(async () => {
    const mfs = await services.graphs
      .getTimeLine()
      .catch(setError)
    if (Array.isArray(mfs)) {
      return mfs
    }
    return []
  }, [reload])

  useEffect(() => {
    if (wfCmd && wfCmd.cmd == LogMessageFlow) {
      const newMF = wfCmd.data
      if (newMF.start) {
        setReload(e => e + 1)
      }
    }
  }, [wfCmd])

  return (
    <div className="">
      <div className="">
        {
          mfs.loading ? <div>Loading</div>
            : <div>
              {
                mfs.value?.map(e => (
                  <div key={e.id}
                    className={`cursor-pointer  px-2 mx-2 my-2 rounded  ${timeline && e.group === timeline.group ? "border-green-500 border-2 border-dashed bg-green-500" : "bg-gray-500"}`}>
                    <EntryTime mf={e} setTimeline={setTimeline} />
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
  id: number,
  name: string,
  hierarchyPointNode: any,
  attributes: any,
}

const TimeLineDetail = ({ timeline }: { timeline: TimeLineSelected }) => {
  const modalRef = useRef(null);
  const wfCmd = useRecoilValue(workflowMonitorAtom)
  const [nodeSelected, setNodeSelected] = useState<NodeSelected | null>();
  const [error, setError] = useState<Error>();
  const mfsCur = useRef<MessageFlow[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [brokers, setBrokers] = useState<Broker[]>([])
  const [delta, setDelta] = useState(0)
  const [treeData, setTreeData] = useState<any>({});
  const [clients, setClients] = useState<Client[]>([])

  const graphS = useAsync(async () => {
    if (timeline.group && timeline.workflow_id) {
      setDelta(delta => 1 - delta)
      const mfs = await services.graphs
        .getGroupID(timeline.group)
        .catch(setError)

      const cl = await services.clients
        .list()
        .catch(setError)

      if (Array.isArray(cl)) {
        setClients(cl)
      }
      // const mfs = await services.graphs
      // .getGroupList(timeline.group, "100")
      // .catch(setError)
      // //@ts-ignore
      // for(const newMF of mfs){
      //   let exist = false
      //   for (let i = 0; i < mfsCur.current.length; i++) {
      //     const mf = mfsCur.current[i]
      //     if (mf.part == newMF.part && mf.parent == newMF.parent
      //       && mf.receiver_id == newMF.receiver_id && mf.receiver_type == newMF.receiver_type) {
      //       mfsCur.current[i] = newMF
      //       exist = true
      //     }
      //   }
      //   if (!exist) {
      //     mfsCur.current.push(newMF)
      //   }
      // }


      const object = await services.workflows.getObjects(timeline.workflow_id)
      for (const t of object.tasks) {
        if (t.clients) {
          // @ts-ignore
          t.clients = JSON.parse(t.clients)
        }
      }

      for (const b of object.brokers) {
        if (b.clients && typeof b.clients == "string") {
          b.clients = JSON.parse(b.clients)
        }
      }

      setTasks(object.tasks)
      setBrokers(object.brokers)

      if (Array.isArray(mfs)) {
        const gs = mfs.filter(e =>
          e.sender_type == ChannelPoint || e.receiver_type == ChannelPoint || e.task_id
        )
        if (mfs.length) {
          mfsCur.current = mfs
          const tr = buildTree(mfsCur.current, clients, tasks, brokers)
          console.log("DATA", tr)
          if (tr) {
            setTreeData(tr)
          }
        }
        return gs
      }
    }
    return []
  }, [timeline])


  useEffect(() => {
    if (wfCmd && wfCmd.cmd == LogMessageFlow && timeline.group == wfCmd.data.group) {
      const newMF = wfCmd.data
      let exist = false
      for (let i = 0; i < mfsCur.current.length; i++) {
        const mf = mfsCur.current[i]
        if (mf.part == newMF.part && mf.parent == newMF.parent
          && mf.receiver_id == newMF.receiver_id && mf.receiver_type == newMF.receiver_type) {
          mfsCur.current[i] = { ...newMF }
          exist = true
        }
      }
      if (!exist) {
        mfsCur.current.push({ ...newMF })
      }
      setTreeData(() => {
        const tree = buildTree(mfsCur.current, clients, tasks, brokers)
        console.log("TREE 2", tree)
        return tree
      })
    }

    if (wfCmd && wfCmd.cmd == ObjectStatusWorkflow) {
      setClients((clients) => {
        for (let i = 0; i < clients.length; i++) {
          const c = clients[i]
          if (wfCmd.object_id == c.id) {
            const newC = [...clients]
            newC[i] = {
              ...c,
              status: wfCmd.status
            }
            setTreeData(() => {
              const tree = buildTree(mfsCur.current, newC, tasks, brokers)
              console.log("TREEE", tree)
              return tree
            })
            return newC
          }
        }
        return clients
      })
    }

  }, [wfCmd])

  // useEffect(() => {
  //   if (groupID) {
  //     // todo add time line
  //   }
  // }, [groupID])

  async function handleClick(hierarchyPointNode: any) {
    // const parts = []
    // const parents = []
    const query: any = {}
    console.log("hierarchyPointNode", hierarchyPointNode)
    if (hierarchyPointNode.data.type == TaskPoint) {
      query.parent = hierarchyPointNode.data.parent
    }

    if (hierarchyPointNode.parent) {
      if (hierarchyPointNode.data.type != TaskPoint) {
        if (hierarchyPointNode.parent.data.part) {
          query.parent = hierarchyPointNode.parent.data.part
        }
      }
      // if (hierarchyPointNode.data.type == TaskPoint && !hierarchyPointNode.data.children.length) {
      //   parents.push(hierarchyPointNode.data.part)
      // }
    }
    if (hierarchyPointNode.data.type == BrokerPoint) {
      const parts = []
      if (hierarchyPointNode.data.children) {
        for (const c of hierarchyPointNode.data.children) {
          parts.push(c.parent)
        }
      }
      query.parts = parts
    } else if (hierarchyPointNode.data.part) {
      query.parts = [hierarchyPointNode.data.part]
    }

    if (!query.parent && hierarchyPointNode.data.parent) {
      query.parent = hierarchyPointNode.data.parent
    }

    query.receiver_id = hierarchyPointNode.data.id
    query.group = timeline.group
    const paths = await services.graphs
      .getParts(query)
      .catch(setError)

    if (paths) {
      setNodeSelected({
        type: hierarchyPointNode.data.type,
        inputs: paths.inputs.reverse(),
        outputs: paths.outputs.reverse(),
        name: hierarchyPointNode.data.name,
        id: hierarchyPointNode.data.id,
        hierarchyPointNode: hierarchyPointNode,
        attributes: hierarchyPointNode.data.attributes
      })
    }
    console.log("PARTS---", paths)
  }

  const statusNode = (attributes: any, status: number) => {
    if (status == -99) {
      return "waring"
    }
    if (attributes.finish) {
      if (attributes.type == TaskPoint) {
        if (attributes.success) {
          return "success"
        }
        return "fault"
      }
      return "success"
    }

    if (attributes.type == BrokerPoint) {
      if (attributes.tracking) {
        try {
          const track = JSON.parse(attributes.tracking)
          if (!track.deliver_flows) {
            return "nodeliver"
          }
        } catch (error) {
          console.error(error)
        }
      }
      // return "success"
    }

    if (attributes.type == TaskPoint) {
      const task = tasks.filter(t => t.id == attributes.id)[0]
      if (task && attributes.client) {
        const client = clients.filter(c => c.name == attributes.client)[0]
        if (client && !client.status) {
          return "offline"
        }
      }
    }

    if (attributes.type == ChannelPoint) {
      return "success"
    }
    // if(attributes.type == BrokerPoint){
    //   const task =  tasks.filter(t=>t.id == attributes.id)[0]
    //   if(task && attributes.client ){
    //     const client = clients.filter(c => c.name == attributes.client)[0]
    //     if(client && !client.status){
    //       return "offline"
    //     }
    //   }
    // }
    // if (!attributes.active) {
    //   return "offline"
    // }
    return "active"
  }


  return (
    <>
      <Modal
        open={nodeSelected != null}
        onClose={() => { setNodeSelected(null) }}
        center
      >
        <div className="min-w-[600px]" >
          {
            nodeSelected && <div>
              <div className="flex py-2">
                <div className="font-bold">{getNameFromType(nodeSelected.type)} : { }</div>
                <div className="ml-2 flex">{nodeSelected.name}<p className="ml-2">{nodeSelected.attributes.client ? `(Run on ${nodeSelected.attributes.client})`:""}</p></div>
              </div>
              <div>
                {
                  nodeSelected.type === BrokerPoint ?
                    <BrokerDetail
                      inputs={nodeSelected.inputs}
                      outputs={nodeSelected.outputs}
                      broker={brokers.filter(b => b.id == nodeSelected.id)[0]}
                      clients={clients}
                      node={nodeSelected.hierarchyPointNode}
                    /> :
                    nodeSelected.type === TaskPoint ?
                      <TaskDetail
                        inputs={nodeSelected.inputs}
                        outputs={nodeSelected.outputs}
                        task={tasks.filter(t => t.id == nodeSelected.id)[0]}
                        clients={clients}
                        node={nodeSelected.hierarchyPointNode}
                      /> :
                      nodeSelected.type === ChannelPoint ?
                        <ChannelDetail
                          inputs={nodeSelected.inputs}
                          outputs={nodeSelected.outputs}
                          id={nodeSelected.id}
                          type={nodeSelected.type}
                        /> :
                        <></>
                }
              </div>
            </div>
          }
        </div>
      </Modal>
      {
        treeData &&
        <Tree
          data={treeData}
          orientation="vertical"
          translate={{
            x: window.innerWidth * 20 / 100 + delta,
            y: 30,
          }}
          renderCustomNodeElement={({ nodeDatum, toggleNode, hierarchyPointNode }) => {
            return (
              <>
                {
                  nodeDatum.attributes?.type == ChannelPoint ?
                    //@ts-ignore
                    <g className={`${statusNode(nodeDatum.attributes, hierarchyPointNode.data.status)}`}>
                      {
                        <rect width="40" height="40" x="-20" onClick={() => { handleClick(hierarchyPointNode) }} />
                      }
                      <text fill="black" strokeWidth="1" x="30" y="10">
                        {nodeDatum.name}
                      </text>
                      <text fill="blue" x="20" dy="30" strokeWidth="1">
                        {"(Channel)"}
                      </text>
                    </g>
                    //@ts-ignore
                    : nodeDatum.attributes?.type == TaskPoint ? <g className={`${statusNode(nodeDatum.attributes, hierarchyPointNode.data.status)}`}>
                      <circle r="20" onClick={() => { handleClick(hierarchyPointNode) }} />
                      <text fill="black" strokeWidth="1" x="30" y="10">
                        {nodeDatum.name}
                      </text>
                      <text fill="black" x="30" dy="30" strokeWidth="1">
                        {"(Task)"}
                      </text>
                    </g>
                      //@ts-ignore
                      : nodeDatum.attributes?.type == BrokerPoint ? <g className={`${statusNode(nodeDatum.attributes, hierarchyPointNode.data.status)}`}>
                        <ellipse rx="25" ry="15" fill="green" onClick={() => { handleClick(hierarchyPointNode) }} />
                        <text fill="black" strokeWidth="1" x="30" y="10">
                          {nodeDatum.name}
                        </text>
                        <text fill="black" x="30" dy="30" strokeWidth="1">
                          {"(Broker)"}
                        </text>
                      </g> : ""
                }
              </>
            )
          }}
        />
      }

    </>
  )
}

type ItemChart = {
  input: any
  flows: {
    request: MessageFlow,
    response?: MessageFlow,
  }[]
}

const validateInput = (input: any) => {
  delete input["run_coun"]
  delete input["offset"]
  delete input["started_at"]
  if (input["finished_at"]) {
    input["finish"] = true
  } else if (input["finished_at"] != undefined) {
    input["finish"] = false
  }
  delete input["finished_at"]

  return input
}

const BrokerDetail = ({ outputs, inputs, broker, clients, node }: { outputs: MessageFlow[], inputs: MessageFlow[], broker: Broker, clients: Client[], node: any }) => {
  var itemCharts: ItemChart = {
    input: {},
    flows: []
  }
  let myOutput = outputs.filter(e => e.receiver_id != -1)
  for (const o of myOutput) {
    if (o.start_input) {
      itemCharts.input = o
      itemCharts.input.value = JSON.parse(o.start_input)
      itemCharts.input.parse_input = JSON.parse(o.start_input)
      break
    }
  }

  if (!itemCharts.input.id) {
    for (let i = 0; i < myOutput.length; i++) {
      if (!myOutput[i].flow || myOutput[i].flow == 2) {
        let request = myOutput[i]
        if (request.sender_name == broker.name) {

        }
        let response, input: any
        for (let j = i + 1; j < myOutput.length; j++) {
          let res = myOutput[j]
          if (request.part == res.part && request.parent == res.parent && res.flow == 1
            && request.sender_id == res.receiver_id
            && request.sender_type == res.receiver_type && res.task_id == request.task_id) {
            response = myOutput[j]
            break;
          }
        }

        for (const e of inputs) {
          if (e.deliver_id == -2) {
            input = inputs[inputs.length - 1]
            break
          } else if (e.part == request.parent && e.broker_group == request.broker_group) {
            input = e
            break
          }
        }

        const msg = JSON.parse(request.message)
        if (request.receiver_type == ChannelPoint) {
          request.value = validateInput(msg)
        } else if (msg && msg.input) {
          if (typeof msg.input == "string") {
            request.value = validateInput(JSON.parse(msg.input))
          }
        }

        itemCharts.flows.push({
          request: request,
          response: response,
        })
        if (input && input.message) {
          try {
            const mi = JSON.parse(input.message)
            if (mi) {
              input.value = validateInput(mi)
              itemCharts.input = input
            }
          } catch (error) {
            console.error(error)
          }
        }

      }
      console.log(itemCharts)
    }
  } else {
    // for (const o of myOutput) {
    //   if (!o.start_input) {
    //     itemCharts.flows.push({
    //       request: o
    //     })
    //   }
    // }
    for (let i = 0; i < myOutput.length; i++) {
      const req = myOutput[i]
      if (req.sender_id == broker.id && req.sender_type == BrokerPoint) {
        let res;
        for (let j = 0; j < myOutput.length; j++) {
          const temp = myOutput[j]
          if (temp.sender_id == req.receiver_id && temp.sender_type == req.receiver_type) {
            res = temp
          }
        }
        const msg = JSON.parse(req.message)
        if (req.receiver_type == ChannelPoint) {
          req.value = validateInput(msg)
        } else if (msg && msg.input) {
          if (typeof msg.input == "string") {
            req.value = validateInput(JSON.parse(msg.input))
          }
        }
        itemCharts.flows.push({
          request: req,
          response: (res && res.status != -99 ? res : undefined)
        })
      }
    }
  }

  if (!itemCharts.input.value) {

  }

  function getRecieve(value: any, node: any) {
    let v: any = {}
    if (value) {
      return value
    }
    if (node && node.parent && node.parent.data.attributes && node.parent.data.attributes.message) {

      try {
        v = JSON.parse(node.parent.data.attributes.message)
      } catch (error) {
        console.error(error)
        v = node.parent.data.attributes.message
      }
    }

    if (node && node.data.attributes.start_input) {

      try {
        v = JSON.parse(node.data.attributes.start_input)
      } catch (error) {
        console.error(error)
        v = node.data.attributes.start_input
      }
    }
    return v
  }

  console.log({ itemCharts })
  return (
    <div>
      <div className="border-2 border-dashed border-blue-500 px-2 py-2">
        <div className="flex items-center">
          <div className="font-bold">Recieve</div>
          <div className="ml-2">
            {
              itemCharts.input.start_input || (node && node.data.attributes.start_input) ? (
                <div className="font-bold text-blue-500">Start by trigger</div>
              ) : itemCharts.input.sender_type == ClientPoint ? (
                <div>{itemCharts.input.task_name}{`(task run on ${itemCharts.input.sender_name})`}</div>
              ) : node && node.parent ? (
                <div>{node.parent.data.name}{`(${getNameFromType(node.parent.data.type)})`}</div>
              ) : (
                <div>{itemCharts.input.sender_name}{`(${getNameFromType(itemCharts.input.sender_type)})`}</div>
              )
            }
          </div>
          <FaArrowRightLong className="ml-2" />
        </div>
        <div>
          {
            itemCharts.input.sender_type == ClientPoint ? <ReactJson
              src={validateInput(getRecieve(itemCharts.input.value, node))}
              name={'result'}
              enableClipboard={false}
            /> : itemCharts.input.parse_input ? (
              <ReactJson
                src={itemCharts.input.parse_input}
                name={false}
                enableClipboard={false}
              />
            ) : (
              <ReactJson
                src={validateInput(getRecieve(itemCharts.input.value, node))}
                name={false}
                enableClipboard={false}
              />
            )
          }

        </div>
      </div>
      <div className="w-full flex justify-center my-3"><FaLongArrowAltDown className="w-6 h-auto" /></div>
      <div className="border-2 border-dashed border-blue-500 px-2 py-2 ">
        {
          broker.flows ?
            <div>
              <div className="flex items-center">
                <div className="font-bold ">Flows</div>
                <TiFlowSwitch className="ml-2" />
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

      <div className="w-full flex justify-center my-3"><FaLongArrowAltDown className="w-6 h-auto" /></div>
      {
        //@ts-ignore
        node && node.data && node.data.children && node.data.children.length == 0 && node.data.attributes.tracking &&
        <div className="border-2 border-dashed border-yellow-500 px-2 py-2 ">
          <div>
            <div className="flex items-center">
              <div className="font-bold ">Tracking</div>
              <CgTrack className="ml-2" />
            </div>
            {node.data.attributes.tracking}
          </div>
        </div>
      }
      {
        itemCharts.flows.map((e, idx) => (
          <div key={idx} className={`border-2 border-dashed ${(itemCharts.input.flow == 2 || e.response || e.request.flow == 2) ? 'border-green-500' : 'border-red-500'} mb-8 px-2 py-2 `}>
            <div className="border-2 border-dashed border-blue-500 px-2 py-2">
              <div className="flex items-center">
                <div className="font-bold">Send</div>
                <FaArrowRightLong className="mx-2" />
                <div>
                  {
                    // daemon
                    itemCharts.input.flow == 2 ? (
                      <div>
                        <div>{e.request.task_name}{`(Run on ${e.request.receiver_name})`}</div>
                      </div>
                    ) : (
                      <div>
                        {
                          e.request.receiver_type == ClientPoint ? (
                            <div>{e.request.task_name}{`(Run on ${e.request.receiver_name})`}</div>
                          ) : (
                            <div>{e.request.receiver_name}{`(${getNameFromType(e.request.receiver_type)})`}</div>
                          )
                        }
                      </div>
                    )
                  }
                </div>
              </div>

              <div className="">
                <ReactJson
                  src={e.request.value ? e.request.value : {}}
                  name={false}
                  enableClipboard={false}
                />
              </div>
            </div>

            <div className={`border-2 border-dashed ${(itemCharts.input.flow == 2 || e.response || e.request.flow == 2) ? 'border-green-500' : 'border-red-500'} px-2 py-2 mt-6`}>
              <div className="flex items-center">
                {e.request && e.request.attempt > 0 ?
                  <div className="mr-2 text-sm">{ "(Attempt "+ e.request.attempt + ")"}</div>
                : ""}
                <div >
                  {
                    itemCharts.input.flow == 2 || e.request.flow == 2
                      ? <div className="text-green-500">Replied</div> : (
                        <div>
                          {
                            e.response ? <div className="text-green-500">Replied 
                            </div>
                              : <div className="text-red-500">No reply</div>
                          }
                        </div>
                      )
                  }
                </div>
              </div>
            </div>

          </div>
        ))
      }
    </div>
  )
}

const mergeInput = (response: any, task: Task) => {
  let payload = {}
  if (task.payload) {
    payload = JSON.parse(task.payload)
  }
  return {
    ...payload,
    ...validateInput(response)
  }
}

const TaskDetail = ({ outputs, inputs, task, clients, node }: { outputs: MessageFlow[], inputs: MessageFlow[], task: Task, clients: Client[], node: any }) => {
  let input = inputs.filter(e => !e.flow)[0]
  if (inputs.length == 1 && !input) {
    input = inputs[0]
  }
  let value: any = ""
  if (input && input.message) {
    try {
      value = JSON.parse(input.message)
    } catch (error) {
      value = input.message
    }
    if (value.input) {
      try {
        value = JSON.parse(value.input)
      } catch (error) {
        value = value.input
      }
    } else {
      try {
        value = JSON.parse(input.message)
      } catch (error) {
        value = input.message
      }
    }
  }
  if (!input) {
    input = inputs.filter(e => e.start_input)[0]
  }

  if (input && input.start_input && input.sender_id == input.receiver_id && input.sender_type == input.receiver_type) {
    try {
      value = JSON.parse(input.start_input)
    } catch (error) {
      value = input.start_input
    }
  }

  console.log({ input, value, outputs })

  const outs = outputs.filter(e => e.flow != 0 && e.cmd != 3).map(e => {
    try {
      e.outobject = JSON.parse(e.message)
    } catch (error) {
      e.outobject = {
        success: true,
        output: e.message
      }
    }
    return e
  })

  return (
    <div>
      <div className="border-blue-500 border-dashed border-2 px-2 py-2">
        <div className="flex">
          <div className="font-bold">Payload</div>
        </div>
        <div className="">
          <ReactJson
            src={task && task.payload ? JSON.parse(task.payload) : {}}
            name={false}
            enableClipboard={false}
          />
        </div>
      </div>
      {/* REQUEST------ */}
      {
        input && input.start_input && input.sender_id == input.receiver_id && input.sender_type == input.receiver_type && (
          <div>
            <div className="flex justify-center py-4"><RiAddLine className="w-6 h-auto font-bold" /></div>
            <div className="border-blue-500 border-dashed border-2 px-2 py-2">
              <div className="font-bold text-blue-500">Start by trigger</div>
              <div className="">
                {
                  typeof value == "string" ? value :
                    <ReactJson src={value ? value : {}} name={false} />
                }
              </div>
            </div>
          </div>
        )
      }

      {/* {
        node && node.data.attributes && node.data.attributes.start_task && (
          <div>
            <div className="flex justify-center py-4"><RiAddLine className="w-6 h-auto font-bold" /></div>
            <div className="border-blue-500 border-dashed border-2 px-2 py-2">
              <div className="font-bold text-blue-500">Start by trigger</div>
              <div className="">
                {
                  typeof node.data.attributes.start_task == "string" ? node.data.attributes.start_task :
                    <ReactJson src={node.data.attributes.start_task ? mergeInput(node.data.attributes.start_task, task) : {}} name={false} />
                }
              </div>
            </div>
          </div>
        )
      } */}

      {
        input && input.sender_type == BrokerPoint && (
          <div>
            <div className="flex justify-center py-4"><RiAddLine className="w-6 h-auto font-bold" /></div>
            <div className="border-blue-500 border-dashed border-2 px-2 py-2">
              <div className="flex">
                <div className="font-bold">Response</div>
                <div className="ml-2">
                  <div className="flex items-center">
                    {input.sender_name}{`(${getNameFromType(input.sender_type)})`}
                    <div className="ml-3"><FaArrowRightLong /></div>
                  </div>
                </div>
              </div>
              <div className="">
                {
                  typeof value == "string" ? value :
                    <ReactJson
                      src={value ? validateInput(value) : {}}
                      name={false}
                      enableClipboard={false}
                    />
                }
              </div>
            </div>
          </div>
        )
      }
      {/* REQUEST------ */}

      <div className="my-2 w-full flex justify-center"><FaLongArrowAltDown className="w-8 h-8" /></div>
      {
        input && input.sender_type == ClientPoint ? (
          <div className="border-green-500 border-dashed border-2 px-2 py-2">
            <div className="flex">
              <div className="font-bold">Input</div>
              <div className="ml-2">
                <div className="flex items-center">
                  {input.sender_name}{`(${getNameFromType(input.sender_type)})`}
                  <div className="ml-3"><FaArrowRightLong /></div>
                </div>
              </div>
            </div>
            <div className="">
              {
                typeof value == "string" ? value :
                  <ReactJson src={value && task && task.payload ? mergeInput(value, task) : {}} name={false} enableClipboard={false}/>
              }
            </div>
          </div>
        ) : input && input.sender_type == BrokerPoint && (
          <div className="border-green-500 border-dashed border-2 px-2 py-2">
            <div className="flex">
              <div className="font-bold">Input</div>
              <div className="ml-2">
                <div className="flex items-center">
                  {input.sender_name}{`(${getNameFromType(input.sender_type)})`}
                  <div className="ml-3"><FaArrowRightLong /></div>
                </div>
              </div>
            </div>
            <div className="">
              {
                typeof value == "string" ? value :
                  <ReactJson src={value && task && task.payload ? mergeInput(value, task) : {}}
                    name={false}
                    enableClipboard={false}
                  />
              }
            </div>
          </div>
        )
      }

      <div className={`border-dashed border-2 border-green-500 mt-6 px-2 py-2 ${outs.filter(e => e.outobject.success == false)[0] ? "border-red-500" : "border-green-500"}`}>
        <div className="font-bold">Output</div>
        <div>
          {
            outs.map(e => (
              <div key={e.id} className="flex items-center">
                <div className="min-w-[22px] text-[10px] mr-2">{formatDate(e.receive_at)}</div>
                {/* <div>{e.outobject.success ? "success" : "fault"}</div> */}
                <p className={`max-w-[500px]  break-all ${e.outobject.success ? "" : "text-red-500"}`}>
                  {e.outobject.output}
                </p>
              </div>
            ))
          }
        </div>
      </div>
    </div>
  )
}

const ChannelDetail = ({ outputs, inputs, id, type }: { outputs: MessageFlow[], inputs: MessageFlow[], id: number, type: number }) => {
  let input = inputs.filter(e => e.receiver_id == id && e.receiver_type == type)[0]
  let output = outputs.filter(e => e.sender_id == id && e.sender_type == type)[0]
  if (inputs.length == 1 && !input) {
    input = inputs[0]
  }

  if(!input){
    input = outputs.filter(e => e.receiver_id == id && e.receiver_type == type)[0]
  }
  let value: any = ""
  if (input && input.message && typeof input.message == "string") {
    try {
      value = JSON.parse(input.message)
    } catch (error) {
      value = input.message
    }
    if (value && value.input && typeof value.input == "string") {
      try {
        value = JSON.parse(value.input)
      } catch (error) {
        value = value.input
      }
    }
  }

  if (!input) {
    for (const o of outputs) {
      if (o.sender_id == -1) {
        input = o
      }
    }
  }

  if (input && input.start_input) {
    try {
      input.start_input = JSON.parse(input.start_input)
      value = input.start_input
    } catch (error) {
      console.error(error)
    }
  }

  console.log({ input })
  return (
    <div>
      {
        input && (
          <div className="border-blue-500 border-dashed border-2 px-2 py-2">
            {
              input.sender_type == BrokerPoint || input.sender_type == TaskPoint ?
                <div className="flex">
                  <div className="font-bold">Recieve</div>
                  <div className="ml-2">
                    <div className="flex items-center">
                      {input.sender_name}{`(${getNameFromType(input.sender_type)})`}
                      <div className="ml-3"><FaArrowRightLong /></div>
                    </div>
                  </div>
                </div>
                : input.start_input &&
                <div className="flex items-center">
                  <div className="font-bold text-blue-500">Start by trigger</div>
                  <div className="ml-3"><FaArrowRightLong /></div>
                </div>
            }

            {
              input.sender_type == ClientPoint ?
                <div className="flex">
                  <div className="font-bold">Send</div>
                  <div className="ml-2">
                    <div className="flex items-center">
                      {input.sender_name}{`(${getNameFromType(input.sender_type)})`}
                      <div className="ml-3"><FaArrowRightLong /></div>
                    </div>
                  </div>
                </div>
                : <div></div>
            }

            <div className="">
              {
                typeof value == "string" ? value :
                  <ReactJson
                    src={value ? validateInput(value) : {}}
                    name={false}
                    enableClipboard={false}
                  />
              }
            </div>
          </div>
        )
      }

      <div className={`border-dashed border-2 mt-6 px-2 py-2 ${output.status == -99 ? 'border-red-500' : 'border-green-500'}`}>

        {
          output.status == -99 ? <div className="text-red-500">No Reply</div> :
            <div className="font-bold text-wrap text-balance">Reply</div>
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

