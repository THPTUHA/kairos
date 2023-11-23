import { atom } from "recoil";

const workflowMonitorAtom = atom<any>({
  key: "workflowMonitorAtom",
  default: null
});

export default workflowMonitorAtom;
