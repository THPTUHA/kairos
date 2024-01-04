import { serverUrl } from "../helper/base"

const LoginPage = () => {
    return (
        <div>
            <a href={`${serverUrl()}/apis/v1/login?user_count_id=kairosweb`} >
                <button>Login</button>
            </a>
        </div>
    )
}

export default LoginPage