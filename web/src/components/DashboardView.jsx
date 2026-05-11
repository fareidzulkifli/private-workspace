'use client';

import { useMemo, useState } from 'react'
import { Link } from 'react-router-dom'
import {
  AlertTriangle,
  ArrowRight,
  CalendarDays,
  CheckCircle2,
  ChevronLeft,
  ChevronRight,
  Circle,
  Clock,
  Edit3,
  Layers,
  Menu,
  Plus,
  Trash2,
  X,
} from 'lucide-react'

const healthIcon = {
  Critical: AlertTriangle,
  'At Risk': Clock,
  Active: Circle,
  Clear: CheckCircle2,
}

const eventColors = [
  { value: 'blue', label: 'Blue' },
  { value: 'green', label: 'Green' },
  { value: 'red', label: 'Red' },
  { value: 'violet', label: 'Violet' },
  { value: 'slate', label: 'Slate' },
]

const sortEvents = (eventList) =>
  [...eventList].sort((a, b) => {
    if (a.event_date !== b.event_date) return a.event_date.localeCompare(b.event_date)
    return new Date(a.created_at || 0) - new Date(b.created_at || 0)
  })

const dateFromKey = (key) => {
  const [year, month, day] = key.split('-').map(Number)
  return new Date(year, month - 1, day)
}

const localDateKey = () => {
  const date = new Date()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

const addDaysKey = (key, days) => {
  const date = dateFromKey(key)
  date.setDate(date.getDate() + days)
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

const formatEventDate = (key) =>
  new Intl.DateTimeFormat('en-US', { month: 'short', day: 'numeric' }).format(dateFromKey(key))

const formatDayTitle = (key) =>
  new Intl.DateTimeFormat('en-US', {
    weekday: 'long',
    month: 'long',
    day: 'numeric',
    year: 'numeric',
  }).format(dateFromKey(key))

const formatHeaderDate = (key) =>
  new Intl.DateTimeFormat('en-US', {
    weekday: 'short',
    month: 'short',
    day: 'numeric',
    year: 'numeric',
  }).format(dateFromKey(key))

function EmptyDashboard({ error }) {
  return (
    <div className="dashboard-empty-panel">
      <div className="dashboard-empty-icon">
        <Layers size={20} />
      </div>
      <h2>{error ? 'Dashboard unavailable' : 'No active portfolio yet'}</h2>
      <p>{error || 'The dashboard will populate once organizations, projects, and tasks exist.'}</p>
    </div>
  )
}

function KpiStrip({ items }) {
  return (
    <div className="dashboard-kpi-strip" aria-label="Key metrics">
      {items.map(item => (
        <div key={item.label} className={`dashboard-kpi dashboard-kpi-${item.tone}`}>
          <strong>{item.value.toString().padStart(2, '0')}</strong>
          <span>{item.label}</span>
        </div>
      ))}
    </div>
  )
}

function FocusPane({ tasks, events, holidays, todayKey, onAddEvent, onEditEvent, onDeleteEvent }) {
  const upcomingEndKey = addDaysKey(todayKey, 30)
  const upcomingEvents = [
    ...events
      .filter(event => event.event_date >= todayKey && event.event_date <= upcomingEndKey)
      .map(event => ({
        ...event,
        date: event.event_date,
        type: 'event',
      })),
    ...(holidays || [])
      .filter(holiday => holiday.date >= todayKey && holiday.date <= upcomingEndKey)
      .map(holiday => ({
        id: holiday.id,
        title: holiday.title,
        date: holiday.date,
        type: 'holiday',
        isMandatory: holiday.isMandatory,
      })),
  ].sort((a, b) => {
    if (a.date !== b.date) return a.date.localeCompare(b.date)
    if (a.type === b.type) return a.title.localeCompare(b.title)
    return a.type === 'holiday' ? -1 : 1
  })

  return (
    <div className="dashboard-left">
      <div className="dashboard-focus-section">
        <div className="dashboard-focus-header">
          <span className="dashboard-section-label">Focus</span>
        </div>
        {tasks.length === 0 ? (
          <div className="dashboard-panel-empty">No urgent tasks open.</div>
        ) : (
          <div className="dashboard-priority-list">
            {tasks.map(task => (
              <Link
                key={task.id}
                to={task.orgSlug ? `/task/org/${task.orgSlug}` : '/dashboard'}
                className="dashboard-priority-row"
              >
                <div className="dashboard-priority-marker" />
                <div className="dashboard-priority-copy">
                  <strong>{task.summary}</strong>
                  <span>{task.orgName} / {task.projectName}</span>
                </div>
                <div className="dashboard-priority-meta">
                  <span className="dashboard-status-pill">{task.status}</span>
                  <small>{task.age}</small>
                </div>
                <ArrowRight size={13} />
              </Link>
            ))}
          </div>
        )}
      </div>

      <div className="dashboard-focus-section">
        <div className="dashboard-focus-header">
          <span className="dashboard-section-label">Events</span>
          <button
            type="button"
            className="btn-ghost dashboard-event-add-btn"
            onClick={() => onAddEvent(todayKey)}
          >
            <Plus size={11} />
            Add
          </button>
        </div>
        {upcomingEvents.length === 0 ? (
          <div className="dashboard-events-empty">No upcoming events.</div>
        ) : (
          <div className="dashboard-event-list">
            {upcomingEvents.map(event => (
              <div key={`${event.type}-${event.id}`} className={`dashboard-event-row ${event.type === 'holiday' ? 'is-holiday' : ''}`}>
                <span className={`dashboard-event-dot ${event.type === 'holiday' ? 'event-holiday' : `event-${event.color}`}`} />
                <div className="dashboard-event-copy">
                  <strong>{event.title}</strong>
                  <span>{formatEventDate(event.date)}{event.type === 'holiday' ? ' / Public holiday' : ''}</span>
                </div>
                {event.type === 'event' && (
                  <div className="dashboard-event-actions">
                    <button
                      type="button"
                      className="btn-ghost"
                      onClick={() => onEditEvent(event)}
                      title="Edit event"
                      aria-label="Edit event"
                    >
                      <Edit3 size={11} />
                    </button>
                    <button
                      type="button"
                      className="btn-ghost"
                      onClick={() => onDeleteEvent(event)}
                      title="Delete event"
                      aria-label="Delete event"
                    >
                      <Trash2 size={11} />
                    </button>
                  </div>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

function CalendarPanel({ calendar, events, onDayClick }) {
  const eventsByDate = useMemo(() => {
    return events.reduce((map, event) => {
      const next = map.get(event.event_date) || []
      next.push(event)
      map.set(event.event_date, next)
      return map
    }, new Map())
  }, [events])
  const monthHref = (monthKey) => monthKey === calendar.todayMonthKey ? '/dashboard' : `/dashboard?month=${monthKey}`

  return (
    <section className="dashboard-panel dashboard-calendar-panel">
      <div className="dashboard-panel-hd dashboard-calendar-hd">
        <div className="dashboard-calendar-heading">
          <span className="dashboard-section-label">Calendar</span>
          <strong className="dashboard-calendar-title">{calendar.label}</strong>
        </div>
        <div className="dashboard-calendar-nav" aria-label="Calendar navigation">
          <Link
            to={monthHref(calendar.previousMonthKey)}
            className="dashboard-calendar-nav-btn"
            title="Previous month"
            aria-label="Previous month"
          >
            <ChevronLeft size={14} />
          </Link>
          <Link
            to={monthHref(calendar.todayMonthKey)}
            className="dashboard-calendar-nav-btn"
            title="Current month"
            aria-label="Current month"
          >
            <CalendarDays size={13} />
          </Link>
          <Link
            to={monthHref(calendar.nextMonthKey)}
            className="dashboard-calendar-nav-btn"
            title="Next month"
            aria-label="Next month"
          >
            <ChevronRight size={14} />
          </Link>
        </div>
      </div>

      <div className="dashboard-calendar">
        <div className="dashboard-calendar-weekdays">
          {calendar.weekdays.map((day, index) => (
            <span key={day} className={index === 0 || index === 6 ? 'is-weekend' : ''}>{day}</span>
          ))}
        </div>
        {calendar.weeks.map((week, index) => (
          <div key={index} className="dashboard-calendar-week">
            {week.map(day => {
              const dayEvents = eventsByDate.get(day.key) || []
              return (
                <button
                  type="button"
                  key={day.key}
                  onClick={() => onDayClick(day)}
                  className={[
                    'dashboard-calendar-day',
                    day.inMonth ? '' : 'is-muted',
                    day.isWeekend ? 'is-weekend' : '',
                    day.isToday ? 'is-today' : '',
                    day.movementCount > 0 ? `has-activity activity-${day.activityLevel}` : '',
                    dayEvents.length > 0 ? 'has-events' : '',
                    day.holidayCount > 0 ? 'has-holidays' : '',
                  ].filter(Boolean).join(' ')}
                  title={`${day.holidayCount} holidays, ${dayEvents.length} events, ${day.movementCount} movements, ${day.completedCount} completed`}
                >
                  <span className="dashboard-calendar-day-number">{day.day}</span>
                  {dayEvents.length > 0 && (
                    <span className="dashboard-calendar-event-stack">
                      {dayEvents.slice(0, 1).map(event => (
                        <span key={event.id} className={`dashboard-calendar-event-line event-${event.color}`}>
                          {event.title}
                        </span>
                      ))}
                    </span>
                  )}
                  <span className="dashboard-calendar-day-footer">
                    {day.movementCount > 0 && (
                      <span className="dashboard-calendar-dots" aria-label={`${day.movementCount} movements`}>
                        {Array.from({ length: Math.min(day.movementCount, 3) }).map((_, dotIndex) => <i key={dotIndex} />)}
                      </span>
                    )}
                    <span className="dashboard-calendar-metrics">
                      {day.holidayCount > 0 && <strong />}
                      {dayEvents.length > 0 && <em />}
                      {day.completedCount > 0 && <b>{day.completedCount}</b>}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        ))}
      </div>
      <div className="dashboard-calendar-legend">
        <span><i /> Movement</span>
        <span><em /> Event</span>
        <span><b /> Done</span>
      </div>
    </section>
  )
}

function HeatmapPanel({ heatmap }) {
  const mobileVisibleStart = Math.max(0, heatmap.weeks.length - 26)
  const monthLabels = useMemo(() => {
    return heatmap.weeks.map((week, i) => {
      if (!week.length) return ''
      const date = dateFromKey(week[0].key)
      if (i === 0) return date.toLocaleString('en-US', { month: 'short' })
      const prev = heatmap.weeks[i - 1]
      if (!prev?.length) return ''
      if (dateFromKey(prev[0].key).getMonth() !== date.getMonth()) {
        return date.toLocaleString('en-US', { month: 'short' })
      }
      return ''
    })
  }, [heatmap.weeks])

  return (
    <section className="dashboard-panel">
      <div className="dashboard-panel-hd">
        <span className="dashboard-section-label">Activity</span>
        <span className="dashboard-heatmap-total">{heatmap.totalMovements} movements</span>
      </div>

      <div className="dashboard-heatmap-wrap">
        <div className="dashboard-heatmap-months">
          {monthLabels.map((label, i) => (
            <span key={i} className={i < mobileVisibleStart ? 'is-mobile-hidden' : ''}>{label}</span>
          ))}
        </div>
        <div className="dashboard-heatmap">
          {heatmap.weeks.map((week, weekIndex) => (
            <div
              key={weekIndex}
              className={`dashboard-heatmap-week ${weekIndex < mobileVisibleStart ? 'is-mobile-hidden' : ''}`}
            >
              {week.map(day => (
                <span
                  key={day.key}
                  className={`dashboard-heatmap-cell level-${day.level}`}
                  title={`${day.key}: ${day.count}`}
                />
              ))}
            </div>
          ))}
        </div>
      </div>

      <div className="dashboard-heatmap-footer">
        <div className="dashboard-heatmap-scale">
          <small>Less</small>
          {[0, 1, 2, 3, 4].map(level => <i key={level} className={`level-${level}`} />)}
          <small>More</small>
        </div>
      </div>
    </section>
  )
}

function ExecutionMix({ mix }) {
  const totalStatus = mix.status.reduce((sum, item) => sum + item.value, 0) || 1
  const totalPriority = mix.priority.reduce((sum, item) => sum + item.value, 0) || 1

  return (
    <section className="dashboard-panel">
      <div className="dashboard-panel-hd">
        <span className="dashboard-section-label">Distribution</span>
      </div>

      <div className="dashboard-mix-group">
        <h3>Status</h3>
        {mix.status.map(item => (
          <div key={item.label} className="dashboard-mix-row">
            <span>{item.label}</span>
            <div><i style={{ width: `${Math.max(4, (item.value / totalStatus) * 100)}%` }} /></div>
            <strong>{item.value}</strong>
          </div>
        ))}
      </div>

      <div className="dashboard-mix-group">
        <h3>Priority</h3>
        {mix.priority.map(item => (
          <div key={item.label} className="dashboard-mix-row">
            <span>{item.label}</span>
            <div><i style={{ width: `${Math.max(4, (item.value / totalPriority) * 100)}%` }} /></div>
            <strong>{item.value}</strong>
          </div>
        ))}
      </div>
    </section>
  )
}

function PortfolioTable({ projects }) {
  return (
    <section className="dashboard-panel dashboard-portfolio-panel">
      <div className="dashboard-panel-hd">
        <span className="dashboard-section-label">Portfolio</span>
      </div>

      <div className="dashboard-portfolio-table">
        <div className="dashboard-portfolio-head">
          <span>Project</span>
          <span>Health</span>
          <span>Active</span>
          <span>Priority</span>
          <span>KIV</span>
          <span>Movement</span>
        </div>
        {projects.map(project => {
          const Icon = healthIcon[project.health] || Circle
          return (
            <Link
              key={project.id}
              to={project.orgSlug ? `/task/org/${project.orgSlug}` : '/dashboard'}
              className="dashboard-portfolio-row"
            >
              <span>
                <strong>{project.name}</strong>
                <small>{project.orgName}</small>
              </span>
              <span className={`dashboard-health dashboard-health-${project.health.toLowerCase().replace(' ', '-')}`}>
                <Icon size={10} />
                {project.health}
              </span>
              <span>{project.activeTasks}</span>
              <span>{project.highPriorityTasks}</span>
              <span>{project.kivTasks}</span>
              <span>{project.lastMovement}</span>
            </Link>
          )
        })}
      </div>
    </section>
  )
}

function EventFormModal({ form, saving, onChange, onClose, onSave }) {
  const isEditing = !!form.id

  return (
    <div
      className="modal-overlay dashboard-modal-overlay"
      onClick={(e) => e.target.classList.contains('modal-overlay') && onClose()}
    >
      <div className="modal-content dashboard-event-modal">
        <div className="dashboard-event-modal-header">
          <div>
            <span className="dashboard-section-label" style={{ marginBottom: 6, display: 'block' }}>Calendar Event</span>
            <h2>{isEditing ? 'Edit Event' : 'Add Event'}</h2>
          </div>
          <button type="button" className="btn-ghost" onClick={onClose} aria-label="Close event form">
            <X size={15} />
          </button>
        </div>

        <div className="dashboard-event-form">
          <label>
            <span>Title</span>
            <input
              autoFocus
              value={form.title}
              onChange={(e) => onChange({ ...form, title: e.target.value })}
              placeholder="Event title"
            />
          </label>

          <label>
            <span>Date</span>
            <input
              type="date"
              value={form.event_date}
              onChange={(e) => onChange({ ...form, event_date: e.target.value })}
            />
          </label>

          <div>
            <span className="dashboard-event-form-label">Color</span>
            <div className="dashboard-event-color-grid">
              {eventColors.map(color => (
                <button
                  key={color.value}
                  type="button"
                  className={`dashboard-event-color ${form.color === color.value ? 'is-selected' : ''} event-${color.value}`}
                  onClick={() => onChange({ ...form, color: color.value })}
                  aria-label={`${color.label} event color`}
                />
              ))}
            </div>
          </div>

          <label>
            <span>Notes</span>
            <textarea
              value={form.notes}
              onChange={(e) => onChange({ ...form, notes: e.target.value })}
              placeholder="Optional context"
              rows={3}
            />
          </label>
        </div>

        <div className="dashboard-event-modal-actions">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
          <button
            type="button"
            className="btn-primary"
            onClick={onSave}
            disabled={saving || !form.title.trim() || !form.event_date}
          >
            {saving ? 'Saving…' : isEditing ? 'Save Event' : 'Create Event'}
          </button>
        </div>
      </div>
    </div>
  )
}

function DeleteEventModal({ event, deleting, onClose, onDelete }) {
  return (
    <div
      className="modal-overlay dashboard-modal-overlay"
      onClick={(e) => e.target.classList.contains('modal-overlay') && onClose()}
    >
      <div className="modal-content dashboard-delete-modal">
        <div className="dashboard-delete-modal-icon">
          <Trash2 size={16} />
        </div>
        <h2>Delete event?</h2>
        <p>{event.title} on {formatEventDate(event.event_date)} will be removed.</p>
        <div className="dashboard-event-modal-actions">
          <button type="button" className="btn-ghost" onClick={onClose}>Cancel</button>
          <button type="button" className="btn-ghost dashboard-delete-btn" onClick={onDelete} disabled={deleting}>
            {deleting ? 'Deleting…' : 'Delete'}
          </button>
        </div>
      </div>
    </div>
  )
}

function DayDetailModal({ day, events, onClose, onAddEvent, onEditEvent, onDeleteEvent }) {
  const holidays = day.holidays || []
  const completedTasks = day.completedTasks || []

  return (
    <div
      className="modal-overlay dashboard-modal-overlay"
      onClick={(e) => e.target.classList.contains('modal-overlay') && onClose()}
    >
      <div className="modal-content dashboard-day-modal">
        <div className="dashboard-event-modal-header">
          <div>
            <span className="dashboard-section-label" style={{ marginBottom: 6, display: 'block' }}>Calendar Day</span>
            <h2>{formatDayTitle(day.key)}</h2>
          </div>
          <button type="button" className="btn-ghost" onClick={onClose} aria-label="Close day details">
            <X size={15} />
          </button>
        </div>

        <div className="dashboard-day-modal-body">
          {holidays.length > 0 && (
            <section>
              <div className="dashboard-day-section-header">
                <h3>Public Holidays</h3>
                <span>{holidays.length.toString().padStart(2, '0')}</span>
              </div>

              <div className="dashboard-holiday-list">
                {holidays.map(holiday => (
                  <div key={holiday.id} className="dashboard-holiday-row">
                    <span className="dashboard-holiday-dot" />
                    <div>
                      <strong>{holiday.title}</strong>
                      <span>{holiday.isMandatory ? 'Mandatory holiday' : holiday.dayOfWeek || 'Malaysia public holiday'}</span>
                    </div>
                  </div>
                ))}
              </div>
            </section>
          )}

          <section>
            <div className="dashboard-day-section-header">
              <h3>Events</h3>
              <button type="button" className="btn-ghost dashboard-event-add-btn" onClick={() => onAddEvent(day.key)}>
                <Plus size={11} />
                Add Event
              </button>
            </div>

            {events.length === 0 ? (
              <div className="dashboard-day-empty">No events scheduled.</div>
            ) : (
              <div className="dashboard-event-list">
                {events.map(event => (
                  <div key={event.id} className="dashboard-event-row">
                    <span className={`dashboard-event-dot event-${event.color}`} />
                    <div className="dashboard-event-copy">
                      <strong>{event.title}</strong>
                      <span>{event.notes || 'No notes'}</span>
                    </div>
                    <div className="dashboard-event-actions">
                      <button type="button" className="btn-ghost" onClick={() => onEditEvent(event)} title="Edit event" aria-label="Edit event">
                        <Edit3 size={11} />
                      </button>
                      <button type="button" className="btn-ghost" onClick={() => onDeleteEvent(event)} title="Delete event" aria-label="Delete event">
                        <Trash2 size={11} />
                      </button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </section>

          <section>
            <div className="dashboard-day-section-header">
              <h3>Completed Tasks</h3>
              <span>{completedTasks.length.toString().padStart(2, '0')}</span>
            </div>

            {completedTasks.length === 0 ? (
              <div className="dashboard-day-empty">No tasks completed on this date.</div>
            ) : (
              <div className="dashboard-completed-list">
                {completedTasks.map(task => (
                  <Link
                    key={task.id}
                    to={task.orgSlug ? `/task/org/${task.orgSlug}` : '/dashboard'}
                    className="dashboard-completed-row"
                  >
                    <CheckCircle2 size={13} />
                    <div>
                      <strong>{task.summary}</strong>
                      <span>{task.orgName} / {task.projectName}</span>
                    </div>
                    <ArrowRight size={13} />
                  </Link>
                ))}
              </div>
            )}
          </section>
        </div>
      </div>
    </div>
  )
}

export default function DashboardView({ data }) {
  const dashboard = data || {}
  const [events, setEvents] = useState(() => sortEvents(dashboard.events || []))
  const [selectedDay, setSelectedDay] = useState(null)
  const [eventForm, setEventForm] = useState(null)
  const [savingEvent, setSavingEvent] = useState(false)
  const [deleteEvent, setDeleteEvent] = useState(null)
  const [deletingEvent, setDeletingEvent] = useState(false)
  const hasDashboard = !dashboard.error && !dashboard.empty && Array.isArray(dashboard.kpis)
  const todayKey = dashboard.todayKey || localDateKey()
  const selectedDayEvents = selectedDay
    ? events.filter(event => event.event_date === selectedDay.key)
    : []

  const openAddEvent = (dateKey = todayKey) => {
    setEventForm({ id: null, title: '', event_date: dateKey, notes: '', color: 'blue' })
  }

  const openEditEvent = (event) => {
    setEventForm({
      id: event.id,
      title: event.title,
      event_date: event.event_date,
      notes: event.notes || '',
      color: event.color || 'blue',
    })
  }

  const handleSaveEvent = async () => {
    if (!eventForm?.title.trim() || !eventForm.event_date) return
    try {
      setSavingEvent(true)
      const url = eventForm.id ? `/api/dashboard/events/${eventForm.id}` : '/api/dashboard/events'
      const res = await fetch(url, {
        method: eventForm.id ? 'PATCH' : 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          title: eventForm.title,
          event_date: eventForm.event_date,
          notes: eventForm.notes,
          color: eventForm.color,
        }),
      })
      const savedEvent = await res.json()
      if (savedEvent.error) throw new Error(savedEvent.error)
      setEvents(prev => sortEvents(eventForm.id
        ? prev.map(event => event.id === savedEvent.id ? savedEvent : event)
        : [...prev, savedEvent]
      ))
      setEventForm(null)
    } catch (err) {
      alert('Event error: ' + err.message)
    } finally {
      setSavingEvent(false)
    }
  }

  const handleDeleteEvent = async () => {
    if (!deleteEvent) return
    try {
      setDeletingEvent(true)
      const res = await fetch(`/api/dashboard/events/${deleteEvent.id}`, { method: 'DELETE' })
      const result = await res.json()
      if (result.error) throw new Error(result.error)
      setEvents(prev => prev.filter(event => event.id !== deleteEvent.id))
      setEventForm(prev => prev?.id === deleteEvent.id ? null : prev)
      setDeleteEvent(null)
    } catch (err) {
      alert('Event error: ' + err.message)
    } finally {
      setDeletingEvent(false)
    }
  }

  return (
    <div className="dashboard-container animate-fade-in">
      <header className="dashboard-header">
        <div className="dashboard-header-left">
          <button
            className="mobile-menu-btn btn-ghost"
            onClick={() => window.dispatchEvent(new Event('toggle-sidebar'))}
            style={{ display: 'none' }}
            aria-label="Open navigation"
          >
            <Menu size={18} />
          </button>
          <span className="dashboard-title">Dashboard</span>
        </div>
        <span className="dashboard-meta">{formatHeaderDate(todayKey)}</span>
      </header>

      {!hasDashboard ? (
        <EmptyDashboard error={dashboard.error} />
      ) : (
        <>
          <KpiStrip items={dashboard.kpis} />

          <div className="dashboard-body">
            <FocusPane
              tasks={dashboard.priorityTasks}
              events={events}
              holidays={dashboard.holidays || []}
              todayKey={todayKey}
              onAddEvent={openAddEvent}
              onEditEvent={openEditEvent}
              onDeleteEvent={setDeleteEvent}
            />

            <div className="dashboard-right">
              <div className="dashboard-right-row">
                <CalendarPanel
                  calendar={dashboard.calendar}
                  events={events}
                  onDayClick={setSelectedDay}
                />
                <ExecutionMix mix={dashboard.executionMix} />
              </div>

              <HeatmapPanel heatmap={dashboard.heatmap} />
              <PortfolioTable projects={dashboard.portfolio} />
            </div>
          </div>

          {selectedDay && (
            <DayDetailModal
              day={selectedDay}
              events={selectedDayEvents}
              onClose={() => setSelectedDay(null)}
              onAddEvent={openAddEvent}
              onEditEvent={openEditEvent}
              onDeleteEvent={setDeleteEvent}
            />
          )}

          {eventForm && (
            <EventFormModal
              form={eventForm}
              saving={savingEvent}
              onChange={setEventForm}
              onClose={() => setEventForm(null)}
              onSave={handleSaveEvent}
            />
          )}

          {deleteEvent && (
            <DeleteEventModal
              event={deleteEvent}
              deleting={deletingEvent}
              onClose={() => setDeleteEvent(null)}
              onDelete={handleDeleteEvent}
            />
          )}
        </>
      )}
    </div>
  )
}
