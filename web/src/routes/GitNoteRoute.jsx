import { useParams } from 'react-router-dom'
import GitNoteLayout from '../components/GitNoteLayout'

export default function GitNoteRoute() {
  const params = useParams()
  const path = params['*'] ? params['*'].split('/').filter(Boolean) : []
  return <GitNoteLayout initialPath={path} />
}
