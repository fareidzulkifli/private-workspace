import { useParams } from 'react-router-dom'
import ShareNoteLayout from '../components/ShareNoteLayout'

export default function ShareRoute() {
  const params = useParams()
  const filePath = params['*'] || null
  return <ShareNoteLayout token={params.token} filePath={filePath} />
}
