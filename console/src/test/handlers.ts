import { http, HttpResponse } from 'msw'
import { setupServer } from 'msw/node'

export const projects = [
  { id: 'prj_a', tenant_id: 'tenant_a', name: 'Support', description: 'Customer answers', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
  { id: 'prj_b', tenant_id: 'tenant_a', name: 'Search', description: 'Product discovery', created_at: '2026-07-11T00:00:00Z', updated_at: '2026-07-11T00:00:00Z' },
]

export const server = setupServer(
  http.get('/v1/projects', () => HttpResponse.json({ projects })),
  http.get('/v1/projects/:projectId', ({ params }) => HttpResponse.json(projects.find((project) => project.id === params.projectId))),
)
