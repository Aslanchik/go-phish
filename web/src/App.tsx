import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { HomePage } from '@/pages/HomePage'
import { InvestigationPage } from '@/pages/InvestigationPage'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/" element={<HomePage />} />
        <Route path="/investigations/:id" element={<InvestigationPage />} />
      </Routes>
    </BrowserRouter>
  )
}
