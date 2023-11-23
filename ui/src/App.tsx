import { Route, Routes } from 'react-router-dom';
import './App.css';
import Verify from './pages/verify';
import HomePage from './pages/home';
import DashBoardPage from './pages/dashboard';
import WorkflowListPage from './pages/workflow/workflowList';
import CommonLayout from './layout/CommonLayout';
import ClientPage from './pages/clients';
import ChannelPage from './pages/channels';
import CertificatePage from './pages/certificates';

function App() {
  return (
    <CommonLayout>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/home" element={<HomePage />} />
        <Route path="/clients" element={<ClientPage />} />
        <Route path="/dashboard" element={<DashBoardPage />} />
        <Route path="/workflows" element={<WorkflowListPage />} />
        <Route path="/verify" element={<Verify />} />
        <Route path="/clients" element={<ClientPage />} />
        <Route path="/channels" element={<ChannelPage />} />
        <Route path="/certificates" element={<CertificatePage />} />
      </Routes>
    </CommonLayout>
  );
}

export default App;