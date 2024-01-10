import { Image, Layout, Menu } from "antd"
import Sider from "antd/es/layout/Sider"
import React, { useEffect, useState } from "react"
import { Link, useLocation } from "react-router-dom";
import { GoWorkflow } from "react-icons/go";
import { GrCertificate, GrVirtualMachine } from "react-icons/gr";
import { RiWechatChannelsLine } from "react-icons/ri";
import { TbLayoutDashboard } from "react-icons/tb";
import { useAsync } from "react-use";
import { services } from "../services";
import { Toast } from "../components/Toast";
import { useRecoilState } from "recoil";
import userInfoAtom from "../recoil/userInfo/atom";
import { Header } from "antd/es/layout/layout";
import { Kairos, SubscriptionState } from "kairos-js";
import workflowMonitorAtom from "../recoil/workflowMonitor/atom";
import { PiFunctionBold } from "react-icons/pi";
import { FcWorkflow } from "react-icons/fc";
import { websocketUrl } from "../helper/base";

const MenuItems = [
    { path: "/workflows", title: "workflows", icon: <GoWorkflow /> },
    { path: "/clients", title: "clients", icon: <GrVirtualMachine /> },
    { path: "/channels", title: "channels", icon: <RiWechatChannelsLine /> },
    { path: "/functions", title: "fuction", icon: <PiFunctionBold /> },
    { path: "/certificates", title: "certificates", icon: <GrCertificate /> },
    { path: "/dashboard/logs", title: "dashboard", icon: <TbLayoutDashboard /> },
    { path: "/graphs", title: "graph", icon: <FcWorkflow /> },
]
function CommonLayout({ children }: { children: React.ReactElement }) {
    const location = useLocation()
    const [error, setError] = useState<Error>();
    const [userInfo, setUserInfo] = useRecoilState(userInfoAtom)
    const [wfCmd, setWfCmd] = useRecoilState(workflowMonitorAtom)
    const [pathSelect, setPathSelect]= useState(location.pathname)
    
    const setup = useAsync(async () => {
        const u = await services.users
            .get()
            .catch(setError)
        if (u && u.id) {
            setUserInfo(u) 
            console.log(u)
            try {
                const channel = `kairosuser-${u.id}`
                const kairos = new Kairos(`${websocketUrl()}/pubsub`, "", {
                    secret_key: localStorage.getItem('accessToken') || "",
                });

                kairos.on('message', function (ctx) {
                    console.log(ctx)
                })

                kairos.on('connected', function (ctx) {
                    console.log("connected")
                });

                kairos.on('connecting', function (ctx) {
                    console.log("Connectting")
                });

                kairos.on('disconnected', function (ctx) {
                    console.log("disconnected")
                });

                kairos.on('publication', function (ctx:any) {
                    // console.log("Reciver message",ctx.data)
                    console.log("CTX----", ctx)
                    const msg = ctx.data
                    if (msg) {
                        ctx.data = JSON.parse(msg)
                    }
                    setWfCmd(ctx)
                })

                kairos.connect();

                // const sub = kairos.newSubscription("chat:index");
                // sub.on('publication', function (ctx) {
                //     console.log(ctx.data)
                // })

                // sub.on('subscribed', function(ctx){
                //     sub.publish({"hello":"baby"})
                // })

                // sub.subscribe();

                // if (sub.state == SubscriptionState.Subscribed){
                //     console.log("RUN HERE")
                //     sub.publish({"hello":"baby"})
                // }
            } catch (error) {
                console.log("Websocket err ", error)
            }

        }
        return u
    }, [])

    useEffect(() => {
        if (error) {
            console.log("ERROR", error)
            Toast.error(error.message)
        }
    }, [error])

    const route = useLocation()
    useEffect(()=>{
        if( route.pathname){
            console.log("Change",route.pathname)
            if(route.pathname.includes("/dashboard")){
                setPathSelect("/dashboard/logs")
            }else{
                setPathSelect( route.pathname.split("?")[0])
            }
           
        }
    },[route])

    return (
        <>
            {
                setup.loading ? <div>Loading</div> :
                    location.pathname == "/" || location.pathname == "/home" || location.pathname == "/verify"
                        ? <>{children}</>
                        : <Layout style={{ minHeight: '100vh' }}>
                            <Sider collapsed={true} >
                                {/* <div className="demo-logo-vertical" >KAIROS</div> */}
                                <Menu
                                    theme="dark"
                                    selectedKeys={[pathSelect]}
                                    mode="inline"
                                    onClick={(key) => { }}
                                >
                                    {
                                        MenuItems.map(item => (
                                            <Menu.Item key={item.path} icon={item.icon} title={item.title} id={item.path}>
                                                <Link to={item.path}> </Link>
                                            </Menu.Item>
                                        ))
                                    }
                                </Menu>
                            </Sider>
                            <Layout>
                                <Header className="flex items-center -ml-10" >
                                    <Image src={userInfo?.avatar} width={40} height={40} className="rounded"/>
                                    <div className="ml-3">{userInfo?.username}</div>
                                </Header>
                                {children}
                            </Layout>
                        </Layout>
            }
        </>
    )
}

export default CommonLayout