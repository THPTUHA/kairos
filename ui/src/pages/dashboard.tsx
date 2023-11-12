import { Link } from "react-router-dom"

const DashBoardPage = () => {
    return (
        <div>
            <Link to={"/workflows"}>
                <button>Workflow</button>
            </Link>
        </div>
    )
}

export default DashBoardPage