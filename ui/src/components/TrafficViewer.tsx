import React, { useCallback, useEffect, useRef, useState } from "react";
import makeStyles from '@mui/styles/makeStyles';
import variables from '../styles/variables.module.scss';
import { Entry } from "../models/entry";
import { useRecoilState, useSetRecoilState } from "recoil";
import { useNavigate, useSearchParams } from "react-router-dom";
import focusedItemAtom from "../recoil/focusedItem/atom";
import focusedStreamAtom from "../recoil/focusedStream/atom";
import focusedContextAtom from "../recoil/focusedContext/atom";
import queryAtom from "../recoil/query/atom";
import queryBuildAtom from "../recoil/queryBuild/atom";
import queryBackgroundColorAtom from "../recoil/queryBackgroundColor/atom";
import { BrokerPointColor, ChannelPointColor, ClientPointColor, ColorYellow, KairosPointColor, TaskPointColor } from "../conts";
import TrafficViewerStyles from "../styles/TrafficViewer.module.sass";
import { EntriesList } from "./entry/EntriesList";
import styles from '../styles/EntriesList.module.sass';
import { EntryDetailed } from "./entry/EntryDetail";
import { Filters } from "./Filters";
import Queryable from "./Queryable";
import { Switch } from "antd";

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
  setView: any;
  actionButtons?: JSX.Element,
}

const DEFAULT_QUERY = "";

export const TrafficViewer: React.FC<TrafficViewerProps> = ({ entries, setView, actionButtons }) => {

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
  const getConnectionIndicator = () => {
    switch (wsReadyState) {
      case WebSocket.OPEN:
        return <div
          className={`${TrafficViewerStyles.indicatorContainer} ${TrafficViewerStyles.greenIndicatorContainer}`}>
          <div className={`${TrafficViewerStyles.indicator} ${TrafficViewerStyles.greenIndicator}`} />
        </div>
      default:
        return <div
          className={`${TrafficViewerStyles.indicatorContainer} ${TrafficViewerStyles.redIndicatorContainer}`}>
          <div className={`${TrafficViewerStyles.indicator} ${TrafficViewerStyles.redIndicator}`} />
        </div>
    }
  }

  const getConnectionTitle = () => {
    switch (wsReadyState) {
      case WebSocket.OPEN:
        return "streaming live traffic"
      default:
        return "streaming paused";
    }
  }

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

  // useInterval(async () => {
  //   setEntries(entriesBuffer.current);
  //   setLastUpdated(Date.now());
  // }, 500, true);

  function changeSwitchView(checked: boolean){
     if(checked){
      setView(0)
     }else{
      setView(1)
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
         <Switch defaultChecked onChange={changeSwitchView} />
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
        </div>
        <div className={classes.details} id="rightSideContainer">
          <EntryDetailed />
        </div>
      </div>}
    </div>
  );
};
