import { useQuery } from '@tanstack/react-query'
import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { projectApi } from '../../api/client'

export function ProjectSwitcher() {
  const { projectId } = useParams()
  const navigate = useNavigate()
  const [open, setOpen] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const wrapper = useRef<HTMLDivElement>(null)
  const trigger = useRef<HTMLButtonElement>(null)
  const options = useRef<Array<HTMLButtonElement | null>>([])
  const projectsQuery = useQuery({ queryKey: ['projects', 'list'], queryFn: projectApi.list })
  const activeQuery = useQuery({ queryKey: ['projects', projectId], queryFn: () => projectApi.get(projectId!), enabled: Boolean(projectId) })

  useEffect(() => {
    if (!open) return
    const close = (event: MouseEvent) => { if (!wrapper.current?.contains(event.target as Node)) setOpen(false) }
    document.addEventListener('mousedown', close)
    return () => document.removeEventListener('mousedown', close)
  }, [open])

  useEffect(() => {
    if (open) options.current[activeIndex]?.focus()
  }, [activeIndex, open])

  const projectItems = projectsQuery.data?.projects ?? []
  const openMenu = (index?: number) => {
    const selectedIndex = projectItems.findIndex((project) => project.id === projectId)
    setActiveIndex(index ?? Math.max(0, selectedIndex))
    setOpen(true)
  }
  const closeMenu = () => { setOpen(false); queueMicrotask(() => trigger.current?.focus()) }
  const selectProject = (index: number) => {
    const project = projectItems[index]
    if (!project) return
    setOpen(false)
    navigate(`/projects/${project.id}/overview`)
  }
  const handleTriggerKeyDown = (event: KeyboardEvent<HTMLButtonElement>) => {
    if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
      event.preventDefault()
      openMenu(event.key === 'ArrowUp' ? Math.max(0, projectItems.length - 1) : undefined)
    }
  }
  const handleOptionKeyDown = (event: KeyboardEvent<HTMLButtonElement>, index: number) => {
    if (event.key === 'Escape') { event.preventDefault(); closeMenu(); return }
    if (event.key === 'Enter' || event.key === ' ') { event.preventDefault(); selectProject(index); return }
    const next = event.key === 'Home' ? 0 : event.key === 'End' ? projectItems.length - 1 : event.key === 'ArrowDown' ? (index + 1) % projectItems.length : event.key === 'ArrowUp' ? (index - 1 + projectItems.length) % projectItems.length : index
    if (next !== index) { event.preventDefault(); setActiveIndex(next) }
  }

  const activeName = activeQuery.data?.name ?? projectsQuery.data?.projects.find((item) => item.id === projectId)?.name ?? '选择项目'
  return <div className="project-switcher" ref={wrapper}>
    <span className="control-label">当前项目</span>
    <button ref={trigger} className="project-trigger" type="button" aria-haspopup="listbox" aria-expanded={open} onKeyDown={handleTriggerKeyDown} onClick={() => open ? closeMenu() : openMenu()}>
      <span className="project-mark">{activeQuery.data || projectId ? activeName.slice(0, 1).toUpperCase() : 'O'}</span><span>{activeName}</span><span className="chevron">⌄</span>
    </button>
    {open ? <div className="project-menu" role="listbox" aria-label="项目">
      {projectItems.map((project, index) => <button ref={(element) => { options.current[index] = element }} role="option" tabIndex={index === activeIndex ? 0 : -1} aria-selected={project.id === projectId} key={project.id} onKeyDown={(event) => handleOptionKeyDown(event, index)} onClick={() => selectProject(index)}>
        <span className="project-mark small">{project.name.slice(0, 1).toUpperCase()}</span><span><strong>{project.name}</strong><small>{project.description || '暂无描述'}</small></span>
      </button>)}
      <button className="new-project-option" onClick={() => { setOpen(false); navigate('/projects/new') }}>＋ 新建项目</button>
    </div> : null}
  </div>
}
