import { atom } from "recoil";

const focusedItemAtom = atom<any>({
  key: "focusedItemAtom",
  default: null
});

export default focusedItemAtom;
