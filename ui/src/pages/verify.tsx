import { useEffect } from "react";
import {useLocation, useNavigate } from "react-router-dom";

const Verify = ()=>{
    const search = useLocation().search;
    const navigate = useNavigate()

    useEffect(()=>{
        const token = new URLSearchParams(search).get('token');
        if(token){
            localStorage.setItem("accessToken", token)
            navigate("/workflows")
        }
    },[search])
    return <div></div>
}

export default Verify