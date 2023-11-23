const ColorGreen = "#d2fad2"
const ColorRed = "#fad6dc"
const ColorYellow = "#f6fad2"
const ColorWhite = "#ffffff"

const KairosPoint = 0
const ClientPoint = 1
const BrokerPoint = 2
const ChannelPoint = 3
const TaskPoint = 4

const Pending = 0
const Delivering = 1
const Running = 2
const Pause = 3

const SetStatusWorkflow = 0
const LogMessageFlow = 1

const PendingDeliver = 0
const SuccessSetTask = 1
const FaultSetTask = 2
const ScuccessTriggerTask = 3
const FaultTriggerTask = 4
const SuccessReceiveInputTaskCmd = 5
const SuccessReceiveOutputTaskCmd = 6
const FaultInputTask = 7


const ReplySetTaskCmd = 0;
const ReplyStartTaskCmd = 1;
const ReplyOutputTaskCmd = 2;
const ReplyInputTaskCmd = 3;

const SetTaskCmd=0;
const TriggerStartTaskCmd=1;
const InputTaskCmd = 2;

const DeliverFlow = 0;
const RecieverFlow = 1;

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
  SetStatusWorkflow,
  LogMessageFlow,

  PendingDeliver,
  SuccessSetTask,
  FaultSetTask,
  ScuccessTriggerTask,
  FaultTriggerTask,
  SuccessReceiveInputTaskCmd,
  SuccessReceiveOutputTaskCmd,
  FaultInputTask,

  ReplySetTaskCmd,
  ReplyStartTaskCmd,
  ReplyOutputTaskCmd,
  ReplyInputTaskCmd,

  SetTaskCmd,
  TriggerStartTaskCmd,
  InputTaskCmd,

  DeliverFlow,
  RecieverFlow,
}
