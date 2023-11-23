import { atom } from "recoil";

const focusedContextAtom = atom({
  key: "focusedContextAtom",
  default: ""
});

export default focusedContextAtom;
