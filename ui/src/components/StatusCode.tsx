import Queryable from "./Queryable";
import { MdDone, MdOutlineDisabledByDefault } from "react-icons/md";
import { FaEquals } from "react-icons/fa6";
import { FaultInputTask, FaultSetTask, FaultTriggerTask, PendingDeliver, SuccessTriggerTask, SuccessReceiveInputTaskCmd, SuccessReceiveOutputTaskCmd, SuccessSetTask } from "../conts";

const Neutral = [PendingDeliver]
const SuccessCode = [SuccessSetTask, SuccessTriggerTask,SuccessReceiveInputTaskCmd, SuccessReceiveOutputTaskCmd, 0]
const FailureCode = [FaultSetTask,FaultTriggerTask,FaultInputTask, -99]

export enum StatusCodeClassification {
  SUCCESS = "success",
  FAILURE = "failure",
  NEUTRAL = "neutral"
}

interface EntryProps {
  statusCode: number,
  statusQuery: string
}

const StatusCode: React.FC<EntryProps> = ({ statusCode, statusQuery }) => {


  return <Queryable
    query={statusQuery}
    displayIconOnMouseOver={true}
    flipped={true}
    iconStyle={{ marginTop: "40px", paddingLeft: "10px" }}
  >
    <span
      title="Status Code"
      className=""
    >
      {SuccessCode.includes(statusCode) ?
        <MdDone className="w-4 h-4 bg-green-500" />
        : FailureCode.includes(statusCode)
          ? <MdOutlineDisabledByDefault className="w-4 h-4 bg-red-500" />
          : <FaEquals className="w-4 h-4 bg-gray-500" />}
    </span>
  </Queryable>
};

export function getClassification(statusCode: number): string {
  let classification = StatusCodeClassification.NEUTRAL;

  if (SuccessCode.includes(statusCode)) {
    classification = StatusCodeClassification.SUCCESS;
  } else if (FailureCode.includes(statusCode)) {
    classification = StatusCodeClassification.FAILURE;
  }

  return classification
}

export default StatusCode;
