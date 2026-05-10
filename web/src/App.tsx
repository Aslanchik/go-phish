import { BrowserRouter, Routes, Route } from 'react-router-dom'
import { Layout } from '@/components/Layout'
import { HomePage } from '@/pages/HomePage'
import { InvestigationPage } from '@/pages/InvestigationPage'

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<HomePage />} />
          <Route path="/investigations/:id" element={<InvestigationPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  )
}
