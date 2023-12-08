import { useRecoilState } from "recoil";
import { Entry, Point } from "../../models/entry";
import focusedItemAtom from "../../recoil/focusedItem/atom";
import StatusCode, { StatusCodeClassification, getClassification } from "../StatusCode";
import ingoingIconSuccess from "../../assets/ingoing-traffic-success.svg"
import outgoingIconSuccess from "../../assets/outgoing-traffic-success.svg"
import ingoingIconFailure from "../../assets/ingoing-traffic-failure.svg"
import outgoingIconFailure from "../../assets/outgoing-traffic-failure.svg"
import ingoingIconNeutral from "../../assets/ingoing-traffic-neutral.svg"
import outgoingIconNeutral from "../../assets/outgoing-traffic-neutral.svg"
import { BrokerPoint, BrokerPointColor, ChannelPoint, ChannelPointColor, ClientPoint, ClientPointColor, ColorGreen, ColorRed, ColorWhite, InputTaskCmd, KairosPoint, KairosPointColor, ReplyInputTaskCmd, ReplyMessageCmd, ReplyOutputTaskCmd, ReplySetTaskCmd, ReplyStartTaskCmd, SetTaskCmd, TaskPoint, TaskPointColor, TriggerStartTaskCmd } from "../../conts";
import React from "react";
import styles from '../../styles/EntriesList.module.sass';
import Queryable from "../Queryable";
import { formatDate } from "../../helper/date";

interface EntryProps {
  entry: Entry;
  style: any;
  headingMode: boolean;
  namespace?: string;
}

export function getColorPoint(point: Point){
  switch(point.type){
    case KairosPoint:
      return KairosPointColor
    case ClientPoint:
      return ClientPointColor
    case BrokerPoint:
      return BrokerPointColor
    case ChannelPoint:
      return ChannelPointColor
    case TaskPoint:
      return TaskPointColor
  }
}
export const EntryItem: React.FC<EntryProps> = ({ entry, style, headingMode, namespace }) => {
  const [focusedItem, setFocusedItem] = useRecoilState(focusedItemAtom);
  const isSelected = focusedItem === entry.id;

  const classification = getClassification(entry.status)
  let ingoingIcon;
  let outgoingIcon;
  switch (classification) {
    case StatusCodeClassification.SUCCESS: {
      ingoingIcon = ingoingIconSuccess;
      outgoingIcon = outgoingIconSuccess;
      break;
    }
    case StatusCodeClassification.FAILURE: {
      ingoingIcon = ingoingIconFailure;
      outgoingIcon = outgoingIconFailure;
      break;
    }
    case StatusCodeClassification.NEUTRAL: {
      ingoingIcon = ingoingIconNeutral;
      outgoingIcon = outgoingIconNeutral;
      break;
    }
  }


  const backgroundColor = classification === StatusCodeClassification.SUCCESS ? ColorGreen
    : classification === StatusCodeClassification.FAILURE ? ColorRed
      : ColorWhite

  const borderStyle = !isSelected ? 'dashed' : 'solid';

  const getMessageCmd = () => {
    if (entry.reply) {
      switch (entry.cmd) {
        case ReplySetTaskCmd:
          return "reply set task"
        case ReplyStartTaskCmd:
          return "reply start task"
        case ReplyOutputTaskCmd:
          return "reply output task"
        case ReplyInputTaskCmd:
          return "reply input task"
        case ReplyMessageCmd:
          return "reply message channel"
      }
    } else {
      switch (entry.cmd) {
        case SetTaskCmd:
          return "set task"
        case TriggerStartTaskCmd:
          return "start task"
        case InputTaskCmd:
          return "send input task"
      }
    }
  }

  return <React.Fragment>
    <div
      id={entry.id + ""}
      className="flex cursor-pointer pl-2"
      onClick={() => {
        if (!setFocusedItem) return;
        setFocusedItem(entry);
      }}
      style={{
        border: isSelected && !headingMode ? `1px ${borderStyle}` : `1px ${borderStyle}`,
        position: !headingMode ? "absolute" : "unset",
        top: style['top'],
        marginTop: !headingMode ? style['marginTop'] : "10px",
        width: !headingMode ? "calc(100% - 25px)" : "calc(100% - 18px)",
        backgroundColor: backgroundColor,
      }}
    >
      <StatusCode statusCode={entry.status} statusQuery={""} />
      <div className="flex items-center w-1/5">
        <Queryable
          query={`workflow.name == "${entry.workflow.name}"`}
          displayIconOnMouseOver={true}
          flipped={true}
          iconStyle={{ marginRight: "16px" }}
        >
          <span className="flex flex-col">
            <span>{entry.workflow.name}</span>
            <span className="text-[10px]">{getMessageCmd()}</span>
          </span>
        </Queryable>
      </div>
      <div className="flex w-2/5">
        {headingMode && namespace ? <Queryable
          query={`dst.namespace == "${namespace}"`}
          displayIconOnMouseOver={true}
          flipped={true}
          iconStyle={{ marginRight: "16px" }}
        >
          <span
            className={`${styles.tcpInfo} ${styles.ip}`}
            title="Namespace"
          >
            {`[${namespace}]`}
          </span>
        </Queryable> : null}
        <Queryable
          query={`src.name == "${entry.src.name}"`}
          displayIconOnMouseOver={true}
          flipped={true}
          iconStyle={{ marginRight: "16px" }}
        >
          <span
            className={`${styles.tcpInfo} ${styles.ip} ${getColorPoint(entry.src)}`}
            title="Source name"
          >
            {entry.src.name}
          </span>
        </Queryable>
        <div className="flex items-center ml-2 mr-2">
          {entry.outgoing ?
            <img
              src={outgoingIcon}
              alt="Outgoing traffic"
              title="Outgoing"
            />
            :
            <img
              src={ingoingIcon}
              alt="Ingoing traffic"
              title="Ingoing"
            />
          }
        </div>
        <Queryable
          query={`dst.name == "${entry.dst.name}"`}
          displayIconOnMouseOver={true}
          flipped={false}
          iconStyle={{ marginTop: "30px", right: "35px", position: "relative" }}
        >
          <span
            className={`${styles.tcpInfo} ${styles.ip} ${getColorPoint(entry.dst)}`}
            title="Destination Name"
          >
            {entry.dst.name}
          </span>
        </Queryable>
      </div>
      <div className="flex items-center">
        {formatDate(entry.timestamp)}
      </div>
    </div>
  </React.Fragment>

}