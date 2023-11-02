import { Route, Routes } from 'react-router-dom';
import './App.css';
import Login from './pages/login';
import Verify from './pages/verify';

function App() {
  return (
   <div>
     <Routes>
      <Route path="/login" element={<Login />} />
      <Route path="/verify" element={<Verify />} />
     </Routes>
   </div>
  );
}

export default App;