import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { projectApi } from '../../api/client'

export function ProjectSwitcher() {
  const { projectId } = useParams()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const wrapper = useRef<HTMLDivElement>(null)
  const projectsQuery = useQuery({ queryKey: ['projects', 'list'], queryFn: projectApi.list })
  const activeQuery = useQuery({ queryKey: ['projects', projectId], queryFn: () => projectApi.get(projectId!), enabled: Boolean(projectId) })

  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent) => { if (!wrapper.current?.contains(event.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  const activeName = activeQuery.data?.name ?? projectsQuery.data?.projects.find((item) => item.id === projectId)?.name ?? '选择项目'
  return <div className="project-switcher" ref={wrapper}>
    <span className="control-label">当前项目</span>
    <button className="project-trigger" type="button" aria-haspopup="listbox" aria-expanded={open} onClick={() => setOpen((value) => !value)}>
      <span className="project-mark">{activeQuery.data || projectId ? activeName.slice(0, 1).toUpperCase() : 'O'}</span><span>{activeName}</span><span className="chevron">⌄</span>
    </button>
    {open ? <div className="project-menu" role="listbox" aria-label="项目">
      {projectsQuery.data?.projects.map((project) => <button role="option" aria-selected={project.id === projectId} key={project.id} onClick={() => { setOpen(false); navigate(`/projects/${project.id}/overview`) }}>
        <span className="project-mark small">{project.name.slice(0, 1).toUpperCase()}</span><span><strong>{project.name}</strong><small>{project.description || '暂无描述'}</small></span>
      </button>)}
      <button className="new-project-option" onClick={() => { setOpen(false); navigate('/projects/new') }}>＋ 新建项目</button>
    </div> : null}
  </div>
}
