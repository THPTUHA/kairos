import { Google } from '@mui/icons-material';
import { Layout } from 'antd';
import { Content, Header } from 'antd/es/layout/layout';
import { Link } from 'react-router-dom';
import { useRecoilValue } from 'recoil';
import userInfoAtom from '../recoil/userInfo/atom';
import { LuWorkflow } from "react-icons/lu";
import { serverUrl } from '../helper/base';

const HomePage = () => {
    const userInfo = useRecoilValue(userInfoAtom)

    return (
        <Layout className="layout">
            <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
                <Link to={"/"}>
                    <div className='text-3xl font-bold'>Kairos</div>
                </Link>
                {
                    !userInfo ?
                        <div >
                            <a href={`${serverUrl()}apis/v1/login?user_count_id=kairosweb`}
                                className='flex items-center cursor-pointer bg-blue-500 rounded px-1 py-1'>
                                <Google>Google</Google>
                                <button className='text-lg'>Login</button>
                            </a>
                        </div>
                        : <span className='cursor-pointer'>
                            <Link to={"/workflows"}>
                                <LuWorkflow className='w-6 h-6' />
                            </Link>
                        </span>
                }
            </Header>
            <Content style={{ padding: '0 50px' }}>
                <p>
                    Vấn đề giải quyết:
                    Kết nối các nền tảng với nhau (Dữ liệu được truyền và nhận giữa các nền tảng thông qua sự điều phối của Kairos)
                    Quản lý các task lập lịch trên thiết bị khác (VD: Khi thực hiện chạy 1 script lập lịch  trên 1 máy tính và nhận kết quả về một thiết bị khác , Kairos sẽ điều phối task đến cho đối tượng được chỉ định để thực hiện task đó ).
                    Xây dựng 1 web, app đơn giản, ứng dụng cá nhân mà không cần thuê server ( Có thể kết nối với 1 máy tính local từ xa để lưu giữ liệu, và chạy các task được điều phối từ Kairos).
                </p>
            </Content>
        </Layout>
    )
}

export default HomePage