import { Route, Routes } from 'react-router-dom';
import './App.css';
import Verify from './pages/verify';
import HomePage from './pages/home';
import DashBoardPage from './pages/dashboard';
import WorkflowListPage from './pages/workflow/workflowList';

function App() {
  return (
    <div>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/dashboard" element={<DashBoardPage />} />
        <Route path="/workflows" element={<WorkflowListPage />} />
        <Route path="/verify" element={<Verify />} />
      </Routes>
    </div>
  );
}

export default App;