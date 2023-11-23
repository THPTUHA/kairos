import { atom } from "recoil";
import { User } from "../../models/user";

const userInfoAtom = atom<User | null>({
  key: "userInfoAtom",
  default: null
});

export default userInfoAtom;
