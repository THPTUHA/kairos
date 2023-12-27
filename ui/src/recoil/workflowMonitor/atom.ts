import { atom } from "recoil";

const workflowMonitorAtom = atom<any|null>({
  key: "workflowMonitorAtom",
  default: null
});

export default workflowMonitorAtom;
