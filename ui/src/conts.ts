const ColorGreen = "#d2fad2"
const ColorRed = "#fad6dc"
const ColorYellow = "#f6fad2"
const ColorWhite = "#ffffff"

const KairosPoint = 0
const ClientPoint = 1
const ChannelPoint = 2
const BrokerPoint = 3
const TaskPoint = 4

const Pending = 0
const Delivering = 1
const Running = 2
const Pause = 3
const Destroying = 4
const Destroyed = 5
const Recovering = 6

const SetStatusWorkflow = 0
const LogMessageFlow = 1
const DestroyWorkflow = 2
const RecoverWorkflow = 3
const ObjectStatusWorkflow = 4

const PendingDeliver = 0
const SuccessSetTask = 1
const FaultSetTask = 2
const SuccessTriggerTask = 3
const FaultTriggerTask = 4
const SuccessReceiveInputTaskCmd = 5
const SuccessReceiveOutputTaskCmd = 6
const FaultInputTask = 7


const ReplySetTaskCmd = 0;
const ReplyStartTaskCmd = 1;
const ReplyOutputTaskCmd = 2;
const ReplyInputTaskCmd = 3;
const ReplyMessageCmd = 4;

const SetTaskCmd=0;
const TriggerStartTaskCmd=1;
const InputTaskCmd = 2;

const DeliverFlow = 0;
const RecieverFlow = 1;

const KairosPointColor =  ` text-[#800080] `;
const ClientPointColor = ` text-blue-500 `;
const BrokerPointColor = ` text-[#FFA500] `;
const ChannelPointColor = `text-white`;
const TaskPointColor = ` text-pink-600 `;

const ViewRealtime= 1;
const ViewHistory = 2;
const ViewTimeLine = 3;

const SuccessCode = [SuccessSetTask, SuccessTriggerTask,SuccessReceiveInputTaskCmd, SuccessReceiveOutputTaskCmd]
const FailureCode = [FaultSetTask,FaultTriggerTask,FaultInputTask]

export {
  ColorGreen,
  ColorRed,
  ColorYellow,
  ColorWhite,
  KairosPoint,
  ClientPoint,
  BrokerPoint,
  ChannelPoint,
  TaskPoint,
  Pending,
  Delivering,
  Running,
  Pause,
  Destroying,
  Recovering,

  SetStatusWorkflow,
  LogMessageFlow,
  DestroyWorkflow,
  RecoverWorkflow,
  ObjectStatusWorkflow,

  PendingDeliver,
  SuccessSetTask,
  FaultSetTask,
  SuccessTriggerTask,
  FaultTriggerTask,
  SuccessReceiveInputTaskCmd,
  SuccessReceiveOutputTaskCmd,
  FaultInputTask,

  ReplySetTaskCmd,
  ReplyStartTaskCmd,
  ReplyOutputTaskCmd,
  ReplyInputTaskCmd,
  ReplyMessageCmd,

  SetTaskCmd,
  TriggerStartTaskCmd,
  InputTaskCmd,

  DeliverFlow,
  RecieverFlow,

  KairosPointColor,
  ClientPointColor,
  BrokerPointColor,
  ChannelPointColor,
  TaskPointColor,

  ViewRealtime,
  ViewHistory,
  ViewTimeLine,

  SuccessCode,
  FailureCode,
}

