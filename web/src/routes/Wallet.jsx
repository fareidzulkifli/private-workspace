import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link, useLocation, useNavigate } from 'react-router-dom'
import {
  AlertTriangle,
  Banknote,
  BarChart3,
  BookOpen,
  CalendarDays,
  ChevronDown,
  ChevronUp,
  CheckCircle2,
  ClipboardList,
  GripVertical,
  Menu,
  Pencil,
  PiggyBank,
  Plus,
  ReceiptText,
  RefreshCw,
  Save,
  Settings,
  Split,
  Tags,
  Trash2,
  Wallet as WalletIcon,
  X,
} from 'lucide-react'
import api from '../lib/api'

const moneyFormatter = new Intl.NumberFormat('en-MY', {
  minimumFractionDigits: 2,
  maximumFractionDigits: 2,
})

const currentMonthKey = () => {
  const date = new Date()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  return `${date.getFullYear()}-${month}`
}

const localDateKey = () => {
  const date = new Date()
  const month = String(date.getMonth() + 1).padStart(2, '0')
  const day = String(date.getDate()).padStart(2, '0')
  return `${date.getFullYear()}-${month}-${day}`
}

const formatMoney = (cents = 0) => `RM ${moneyFormatter.format((cents || 0) / 100)}`

const moneyInputValue = (cents = 0) => ((cents || 0) / 100).toFixed(2)

const parseCents = (value, label) => {
  const raw = String(value ?? '').replace(/rm/ig, '').replace(/,/g, '').trim()
  if (raw === '') return 0
  if (!/^-?\d+(\.\d{0,2})?$/.test(raw)) {
    throw new Error(`${label} must be a valid amount`)
  }
  const negative = raw.startsWith('-')
  const normalized = negative ? raw.slice(1) : raw
  const [whole, decimal = ''] = normalized.split('.')
  const cents = (Number(whole) * 100) + Number(decimal.padEnd(2, '0'))
  if (!Number.isSafeInteger(cents)) {
    throw new Error(`${label} is too large`)
  }
  return negative ? -cents : cents
}

const validateAmount = (value, label, { required = false, positive = false, nonNegative = false } = {}) => {
  const raw = String(value ?? '').trim()
  if (raw === '') return required ? `${label} is required` : ''
  try {
    const cents = parseCents(raw, label)
    if (positive && cents <= 0) return `${label} must be greater than zero`
    if (nonNegative && cents < 0) return `${label} must be zero or greater`
  } catch (err) {
    return err.message
  }
  return ''
}

const validateName = (value, label) => String(value ?? '').trim() ? '' : `${label} is required`

const validateDate = (value, label, required = true) => {
  const raw = String(value ?? '').trim()
  if (!raw) return required ? `${label} is required` : ''
  return /^\d{4}-\d{2}-\d{2}$/.test(raw) ? '' : `${label} must use YYYY-MM-DD`
}

const validateMonth = (value, label, required = true) => {
  const raw = String(value ?? '').trim()
  if (!raw) return required ? `${label} is required` : ''
  return /^\d{4}-\d{2}$/.test(raw) ? '' : `${label} must use YYYY-MM`
}

const validateInteger = (value, label, { required = false, min = null, max = null } = {}) => {
  const raw = String(value ?? '').trim()
  if (!raw) return required ? `${label} is required` : ''
  if (!/^-?\d+$/.test(raw)) return `${label} must be a whole number`
  const parsed = Number(raw)
  if (min !== null && parsed < min) return `${label} must be ${min} or greater`
  if (max !== null && parsed > max) return `${label} must be ${max} or less`
  return ''
}

const hasErrors = (errors) => Object.values(errors).some(Boolean)

const visibleFieldError = (errors, touched, field) => (
  touched?.submit || touched?.[field] ? errors[field] : ''
)

const categoryIds = (categories = []) => categories.map(category => category.id)

const templateCategoryIds = (template) => (
  template.default_category_ids || categoryIds(template.default_categories || [])
)

const allocationTemplateSnapshot = (template) => ({
  name: template.name,
  amount_input: template.amount_input,
  default_amount_cents: template.default_amount_cents,
  type: template.type,
  carry_forward: template.carry_forward,
  active: template.active,
  default_category_ids: template.default_category_ids,
  default_categories: template.default_categories,
})

const incomeTemplateSnapshot = (template) => ({
  name: template.name,
  amount_input: template.amount_input,
  default_amount_cents: template.default_amount_cents,
  default_day_input: template.default_day_input,
  default_day: template.default_day,
  active: template.active,
})

const categorySnapshot = (category) => ({
  name: category.name,
  active: category.active,
})

const withFieldError = (error) => ({
  'aria-invalid': !!error,
  className: error ? 'is-invalid' : undefined,
})

const splitRoleLabel = (transaction) => {
  if (transaction?.split_role === 'parent' || transaction?.split_child_count > 0) return 'Split parent'
  if (transaction?.split_role === 'child' || transaction?.parent_transaction_id) return 'Split child'
  return ''
}

const isSplitParent = (transaction) => transaction?.split_role === 'parent' || transaction?.split_child_count > 0
const isSplitChild = (transaction) => transaction?.split_role === 'child' || !!transaction?.parent_transaction_id

const orderedCategoriesForAllocation = (allocation, categories = []) => {
  const defaults = allocation?.default_categories || []
  const ordered = []
  const seen = new Set()
  const add = (category) => {
    if (!category || seen.has(category.id)) return
    seen.add(category.id)
    ordered.push(category)
  }
  defaults.forEach(add)
  categories.filter(category => category.system_key === 'unsorted').forEach(add)
  categories.forEach(add)
  return ordered
}

const formatMonthLabel = (monthKey) => {
  const [year, month] = monthKey.split('-').map(Number)
  return new Intl.DateTimeFormat('en-US', { month: 'long', year: 'numeric' }).format(new Date(year, month - 1, 1))
}

const typeLabel = (type) => ({
  fixed: 'Fixed',
  flexible: 'Flexible',
  sinking_fund: 'Sinking Fund',
  one_off: 'One-Off',
}[type] || 'Flexible')

function WalletModal({ title, onClose, children, wide = false }) {
  return (
    <div className="wallet-modal-backdrop" role="presentation">
      <div className={`wallet-modal ${wide ? 'wallet-modal-wide' : ''}`} role="dialog" aria-modal="true" aria-label={title}>
        <div className="wallet-modal-header">
          <strong>{title}</strong>
          <button type="button" className="btn-ghost" onClick={onClose} title="Close">
            <X size={14} />
          </button>
        </div>
        {children}
      </div>
    </div>
  )
}

function SummaryMetric({ label, value, tone, icon: Icon }) {
  return (
    <div className={`wallet-metric ${tone ? `wallet-metric-${tone}` : ''}`}>
      <div>
        <span>{label}</span>
        <strong>{formatMoney(value)}</strong>
      </div>
      {Icon && <Icon size={16} />}
    </div>
  )
}

function FieldError({ error }) {
  if (!error) return null
  return <small className="wallet-field-error">{error}</small>
}

function WalletField({ children, error, className = '' }) {
  return (
    <div className={`wallet-field ${className}`}>
      {children}
      <FieldError error={error} />
    </div>
  )
}

function AllocationCategoryButton({ count, onClick }) {
  return (
    <button type="button" className="btn-ghost wallet-category-modal-btn" onClick={onClick}>
      <Tags size={13} />
      <span>{count} categor{count === 1 ? 'y' : 'ies'}</span>
    </button>
  )
}

function RecentTransactionChips({ chips, disabled, onSelect }) {
  if (!chips.length) return null
  return (
    <div className="wallet-recent-chip-list" aria-label="Recent transaction categories">
      {chips.map(chip => (
        <button
          key={`${chip.allocationId}:${chip.categoryId}`}
          type="button"
          className="wallet-recent-chip"
          onClick={() => onSelect(chip)}
          disabled={disabled}
          title={chip.label}
        >
          <span>{chip.label}</span>
        </button>
      ))}
    </div>
  )
}

function TransactionCaptureForm({
  summary,
  saving,
  monthClosed,
  activeAllocations,
  quickCategoryOptions,
  transactionForm,
  setTransactionForm,
  transactionDisplayErrors,
  transactionErrors,
  transactionMoreOpen,
  setTransactionMoreOpen,
  touchTransaction,
  submitTransaction,
  recentChips,
  onRecentChipSelect,
  amountInputRef,
  variant = 'desktop',
}) {
  const isMobile = variant !== 'desktop'
  const categories = summary?.categories || []
  const captureDisabled = monthClosed || activeAllocations.length === 0
  const formClassName = `wallet-transaction-form wallet-transaction-form-${variant}`
  const moreId = `wallet-capture-more-${variant}`
  const handleAllocationChange = (event) => {
    const allocationId = event.target.value
    const allocation = activeAllocations.find(item => item.id === allocationId)
    const categoryOptions = orderedCategoriesForAllocation(allocation, categories)
    setTransactionForm(prev => ({
      ...prev,
      allocationId,
      categoryId: categoryOptions.some(category => category.id === prev.categoryId)
        ? prev.categoryId
        : categoryOptions[0]?.id || '',
    }))
  }
  const secondaryFields = (
    <>
      <label>
        <span>Date</span>
        <WalletField error={transactionDisplayErrors.date}>
          <input
            type="date"
            value={transactionForm.date}
            onChange={event => setTransactionForm(prev => ({ ...prev, date: event.target.value }))}
            onBlur={() => touchTransaction('date')}
            disabled={captureDisabled}
            {...withFieldError(transactionDisplayErrors.date)}
          />
        </WalletField>
      </label>
      <label className="wallet-note-field">
        <span>Note</span>
        <input
          value={transactionForm.note}
          onChange={event => setTransactionForm(prev => ({ ...prev, note: event.target.value }))}
          placeholder="Optional"
          disabled={captureDisabled}
        />
      </label>
      <label className="wallet-check-field">
        <input
          type="checkbox"
          checked={transactionForm.rounded}
          onChange={event => setTransactionForm(prev => ({ ...prev, rounded: event.target.checked }))}
          disabled={captureDisabled}
        />
        <span>Rounded</span>
      </label>
    </>
  )

  return (
    <form onSubmit={submitTransaction} className={formClassName}>
      {monthClosed && (
        <div className="wallet-capture-disabled-state">This month is closed. Reopen it before adding transactions.</div>
      )}
      {!monthClosed && activeAllocations.length === 0 && (
        <div className="wallet-capture-disabled-state">Create an active allocation before adding transactions.</div>
      )}
      <label className="wallet-amount-field">
        <span>Amount</span>
        <WalletField error={transactionDisplayErrors.amount}>
          <input
            ref={amountInputRef}
            value={transactionForm.amount}
            onChange={event => setTransactionForm(prev => ({ ...prev, amount: event.target.value }))}
            onBlur={() => touchTransaction('amount')}
            placeholder="0.00"
            inputMode="decimal"
            disabled={captureDisabled}
            {...withFieldError(transactionDisplayErrors.amount)}
          />
        </WalletField>
      </label>
      <label>
        <span>Allocation</span>
        <WalletField error={transactionDisplayErrors.allocation}>
          <select
            value={transactionForm.allocationId}
            onChange={handleAllocationChange}
            onBlur={() => touchTransaction('allocation')}
            disabled={captureDisabled}
            {...withFieldError(transactionDisplayErrors.allocation)}
          >
            {activeAllocations.map(allocation => (
              <option key={allocation.id} value={allocation.id}>{allocation.name}</option>
            ))}
          </select>
        </WalletField>
      </label>
      <label>
        <span>Category</span>
        <WalletField error={transactionDisplayErrors.category}>
          <select
            value={transactionForm.categoryId}
            onChange={event => setTransactionForm(prev => ({ ...prev, categoryId: event.target.value }))}
            onBlur={() => touchTransaction('category')}
            disabled={captureDisabled || categories.length === 0}
            {...withFieldError(transactionDisplayErrors.category)}
          >
            {quickCategoryOptions.map(category => (
              <option key={category.id} value={category.id}>{category.name}</option>
            ))}
          </select>
        </WalletField>
      </label>
      {isMobile && (
        <RecentTransactionChips
          chips={recentChips}
          disabled={captureDisabled}
          onSelect={onRecentChipSelect}
        />
      )}
      {isMobile ? (
        <>
          <button
            type="button"
            className="wallet-capture-more-toggle"
            onClick={() => setTransactionMoreOpen(prev => !prev)}
            aria-expanded={transactionMoreOpen}
            aria-controls={moreId}
          >
            {transactionMoreOpen ? <ChevronUp size={14} /> : <ChevronDown size={14} />}
            More
          </button>
          {transactionMoreOpen && (
            <div id={moreId} className="wallet-capture-more">
              {secondaryFields}
            </div>
          )}
        </>
      ) : secondaryFields}
      <button type="submit" className="btn-primary" disabled={saving || captureDisabled || hasErrors(transactionErrors)}>
        <Plus size={14} />
        Add Transaction
      </button>
    </form>
  )
}

function RecentTransactionsPanel({
  transactions = [],
  saving,
  monthClosed,
  transactionAmountEdit,
  transactionAmountEditDisplayErrors,
  transactionAmountEditErrors,
  startTransactionAmountEdit,
  updateTransactionAmountEdit,
  setTransactionAmountEdit,
  saveTransactionAmountEdit,
  openSplitModal,
  openSplitDetail,
  deleteTransaction,
  className = '',
}) {
  return (
    <section className={`wallet-panel wallet-recent-panel ${className}`}>
      <div className="wallet-panel-header">
        <div>
          <span className="wallet-section-label">Recent</span>
          <strong>Transactions</strong>
        </div>
      </div>
      <div className="wallet-transaction-list">
        {transactions.length === 0 ? (
          <div className="wallet-empty-row">No transactions captured yet.</div>
        ) : transactions.map(transaction => {
          const label = splitRoleLabel(transaction)
          const editingAmount = transactionAmountEdit?.id === transaction.id
          return (
            <div key={transaction.id} className="wallet-transaction-row">
              <span className="wallet-transaction-date">{transaction.date}</span>
              <div>
                <strong>{transaction.note || transaction.category_name}</strong>
                <span>{transaction.allocation_name} / {transaction.category_name}{transaction.rounded ? ' / Rounded' : ''}{label ? ` / ${label}` : ''}</span>
              </div>
              <strong>{formatMoney(transaction.amount_cents)}</strong>
              <button
                type="button"
                className="btn-ghost wallet-transaction-action-btn"
                onClick={() => startTransactionAmountEdit(transaction)}
                disabled={saving || monthClosed || isSplitChild(transaction)}
                title={isSplitChild(transaction) ? 'Split child amount is locked' : 'Edit transaction amount'}
                aria-label={isSplitChild(transaction) ? 'Split child amount is locked' : 'Edit transaction amount'}
              >
                <Pencil size={15} />
              </button>
              <button
                type="button"
                className="btn-ghost wallet-transaction-action-btn"
                onClick={() => isSplitParent(transaction) ? openSplitDetail(transaction) : openSplitModal(transaction)}
                disabled={saving || isSplitChild(transaction) || (!isSplitParent(transaction) && monthClosed)}
                title={isSplitParent(transaction) ? 'View split group' : 'Split transaction'}
                aria-label={isSplitParent(transaction) ? 'View split group' : 'Split transaction'}
              >
                <Split size={15} />
              </button>
              <button
                type="button"
                className="btn-ghost wallet-danger-btn wallet-transaction-action-btn"
                onClick={() => deleteTransaction(transaction)}
                disabled={saving || monthClosed || isSplitChild(transaction)}
                title={isSplitChild(transaction) ? 'Split child delete is blocked' : 'Delete transaction'}
                aria-label={isSplitChild(transaction) ? 'Split child delete is blocked' : 'Delete transaction'}
              >
                <Trash2 size={15} />
              </button>
              {editingAmount && (
                <form onSubmit={saveTransactionAmountEdit} className="wallet-transaction-amount-form">
                  <WalletField error={transactionAmountEditDisplayErrors.amount}>
                    <input
                      value={transactionAmountEdit.amount}
                      onChange={event => updateTransactionAmountEdit({ amount: event.target.value })}
                      onBlur={() => updateTransactionAmountEdit({ touched: { ...(transactionAmountEdit.touched || {}), amount: true } })}
                      inputMode="decimal"
                      aria-label="Transaction amount"
                      {...withFieldError(transactionAmountEditDisplayErrors.amount)}
                    />
                  </WalletField>
                  <button type="button" className="btn-ghost" onClick={() => setTransactionAmountEdit(null)} disabled={saving}>Cancel</button>
                  <button type="submit" className="btn-primary" disabled={saving || monthClosed || hasErrors(transactionAmountEditErrors)}>
                    <Save size={13} />
                    Save
                  </button>
                </form>
              )}
            </div>
          )
        })}
      </div>
    </section>
  )
}

function WalletMobileCaptureSheet({ open, onClose, children }) {
  if (!open) return null
  return (
    <div
      className="wallet-capture-sheet-backdrop"
      role="presentation"
      onMouseDown={event => {
        if (event.target === event.currentTarget) onClose()
      }}
    >
      <section
        className="wallet-capture-sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby="wallet-capture-sheet-title"
      >
        <div className="wallet-capture-sheet-header">
          <div>
            <span className="wallet-section-label">Wallet</span>
            <strong id="wallet-capture-sheet-title">Quick Capture</strong>
          </div>
          <button type="button" className="btn-ghost" onClick={onClose} aria-label="Close capture">
            <X size={15} />
          </button>
        </div>
        {children}
      </section>
    </div>
  )
}

function WalletNotice({ notice, saving, onUndo }) {
  if (!notice) return null
  const message = notice.label
    ? `Added ${formatMoney(notice.amountCents)} to ${notice.label}`
    : `Added ${formatMoney(notice.amountCents)}`
  return (
    <div className="wallet-transaction-notice" role="status">
      <CheckCircle2 size={15} />
      <span>{message}</span>
      {notice.id && (
        <button type="button" className="btn-ghost" onClick={onUndo} disabled={saving}>
          Undo
        </button>
      )}
    </div>
  )
}

function AllocationCategoryModal({ title, categories, selectedIds, onChange, onClose }) {
  const selected = selectedIds || []
  return (
    <WalletModal title={title} onClose={onClose}>
      <div className="wallet-category-modal-body">
        <div className="wallet-category-modal-grid">
          {categories.map(category => (
            <label key={category.id} className="wallet-category-modal-option">
              <input
                type="checkbox"
                checked={selected.includes(category.id)}
                onChange={event => {
                  const next = event.target.checked
                    ? [...selected, category.id]
                    : selected.filter(id => id !== category.id)
                  onChange(next)
                }}
              />
              <span>{category.name}</span>
            </label>
          ))}
        </div>
        <div className="wallet-modal-actions">
          <button type="button" className="btn-primary" onClick={onClose}>Done</button>
        </div>
      </div>
    </WalletModal>
  )
}

function SettingsSortHandle({ label, disabled, onPointerDown, onPointerMove, onPointerUp, onKeyDown }) {
  return (
    <button
      type="button"
      className="btn-ghost wallet-sort-handle"
      disabled={disabled}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
      onKeyDown={onKeyDown}
      aria-label={label}
      title={label}
    >
      <GripVertical size={15} />
    </button>
  )
}

function WalletSettingsView({
  settings,
  saving,
  templateForm,
  setTemplateForm,
  incomeTemplateForm,
  setIncomeTemplateForm,
  categoryForm,
  setCategoryForm,
  updateSettingsList,
  submitAllocationTemplate,
  saveAllocationTemplate,
  deleteAllocationTemplate,
  submitIncomeTemplate,
  saveIncomeTemplate,
  deleteIncomeTemplate,
  submitCategory,
  saveCategory,
  deleteCategory,
  reorderSettingsList,
}) {
  const categories = settings?.categories || []
  const activeCategories = categories.filter(category => category.active || category.system_key === 'unsorted')
  const [categoryPicker, setCategoryPicker] = useState(null)
  const [rowSaveAttempts, setRowSaveAttempts] = useState({})
  const [newFormTouched, setNewFormTouched] = useState({})
  const [sortDrag, setSortDrag] = useState(null)
  const sortDragRef = useRef(null)
  const templateFormErrors = {
    name: validateName(templateForm.name, 'Allocation template name'),
    amount: validateAmount(templateForm.amount, 'Default amount', { nonNegative: true }),
  }
  const templateFormDisplayErrors = {
    name: visibleFieldError(templateFormErrors, newFormTouched.allocation || {}, 'name'),
    amount: visibleFieldError(templateFormErrors, newFormTouched.allocation || {}, 'amount'),
  }
  const incomeTemplateFormErrors = {
    name: validateName(incomeTemplateForm.name, 'Income template name'),
    amount: validateAmount(incomeTemplateForm.amount, 'Default amount', { nonNegative: true }),
    defaultDay: validateInteger(incomeTemplateForm.defaultDay, 'Default day', { min: 1, max: 31 }),
  }
  const incomeTemplateFormDisplayErrors = {
    name: visibleFieldError(incomeTemplateFormErrors, newFormTouched.income || {}, 'name'),
    amount: visibleFieldError(incomeTemplateFormErrors, newFormTouched.income || {}, 'amount'),
    defaultDay: visibleFieldError(incomeTemplateFormErrors, newFormTouched.income || {}, 'defaultDay'),
  }
  const categoryFormErrors = {
    name: validateName(categoryForm.name, 'Category name'),
  }
  const categoryFormDisplayErrors = {
    name: visibleFieldError(categoryFormErrors, newFormTouched.category || {}, 'name'),
  }
  const pickedTemplate = categoryPicker?.templateId
    ? (settings?.allocation_templates || []).find(template => template.id === categoryPicker.templateId)
    : null
  const categoryPickerSelectedIds = categoryPicker?.kind === 'template-form'
    ? (templateForm.defaultCategoryIds || [])
    : (pickedTemplate ? templateCategoryIds(pickedTemplate) : [])
  const categoryPickerTitle = categoryPicker?.kind === 'template-form'
    ? 'New Template Categories'
    : `${pickedTemplate?.name || 'Allocation'} Categories`
  const setNewTouched = (form, field) => {
    setNewFormTouched(prev => ({
      ...prev,
      [form]: { ...(prev[form] || {}), [field]: true },
    }))
  }
  const rowAttemptKey = (key, id) => `${key}:${id}`
  const rowAttempted = (key, id) => !!rowSaveAttempts[rowAttemptKey(key, id)]
  const markRowSaveAttempt = (key, id) => {
    setRowSaveAttempts(prev => ({ ...prev, [rowAttemptKey(key, id)]: true }))
  }
  const clearRowSaveAttempt = (key, id) => {
    setRowSaveAttempts(prev => {
      const next = { ...prev }
      delete next[rowAttemptKey(key, id)]
      return next
    })
  }
  const snapshotForSettingsItem = (key, item) => {
    if (item._wallet_original) return item._wallet_original
    if (key === 'allocation_templates') return allocationTemplateSnapshot(item)
    if (key === 'income_templates') return incomeTemplateSnapshot(item)
    return categorySnapshot(item)
  }
  const editSettingsItem = (key, item, patch) => {
    updateSettingsList(key, item.id, {
      _wallet_original: snapshotForSettingsItem(key, item),
      ...patch,
    })
    clearRowSaveAttempt(key, item.id)
  }
  const revertSettingsItem = (key, item) => {
    if (!item?._wallet_original) return
    updateSettingsList(key, item.id, {
      ...item._wallet_original,
      _wallet_original: null,
    })
    clearRowSaveAttempt(key, item.id)
  }
  const handleSettingsRowBlur = (key, item, event) => {
    const row = event.currentTarget
    window.setTimeout(() => {
      if (key === 'allocation_templates' && categoryPicker?.templateId === item.id) return
      if (row.contains(document.activeElement)) return
      revertSettingsItem(key, item)
    }, 0)
  }
  const handleSettingsRowKeyDown = (key, item, event) => {
    if (event.key !== 'Escape') return
    event.preventDefault()
    revertSettingsItem(key, item)
    document.activeElement?.blur?.()
  }
  const handleSaveSettingsRow = async (key, item, errors, save) => {
    markRowSaveAttempt(key, item.id)
    if (hasErrors(errors)) return
    const saved = await save(item)
    if (saved) {
      updateSettingsList(key, item.id, { _wallet_original: null })
      clearRowSaveAttempt(key, item.id)
    }
  }
  const settingItems = (key) => settings?.[key] || []
  const sortedIDsAfterMove = (key, sourceId, targetId) => {
    const items = settingItems(key)
    const from = items.findIndex(item => item.id === sourceId)
    const to = items.findIndex(item => item.id === targetId)
    if (from < 0 || to < 0 || from === to) return []
    const next = [...items]
    const [moved] = next.splice(from, 1)
    next.splice(to, 0, moved)
    return next.map(item => item.id)
  }
  const moveSettingsItem = (key, sourceId, targetId) => {
    const orderedIds = sortedIDsAfterMove(key, sourceId, targetId)
    if (orderedIds.length === 0) return
    reorderSettingsList(key, orderedIds)
  }
  const sortRowClass = (key, id, baseClass) => {
    if (sortDrag?.key !== key) return baseClass
    if (sortDrag.sourceId === id) return `${baseClass} wallet-row-drag-source`
    if (sortDrag.targetId === id) return `${baseClass} wallet-row-drag-target`
    return baseClass
  }
  const beginSortDrag = (key, item, event) => {
    if (saving || (event.pointerType === 'mouse' && event.button !== 0)) return
    event.preventDefault()
    if (item?._wallet_original) revertSettingsItem(key, item)
    const drag = { key, sourceId: item.id, targetId: item.id }
    sortDragRef.current = drag
    setSortDrag(drag)
    event.currentTarget.setPointerCapture?.(event.pointerId)
  }
  const updateSortDrag = (event) => {
    const drag = sortDragRef.current
    if (!drag) return
    const row = document
      .elementFromPoint(event.clientX, event.clientY)
      ?.closest(`[data-wallet-sort-key="${drag.key}"][data-wallet-sort-id]`)
    const targetId = row?.dataset?.walletSortId
    if (!targetId || targetId === drag.targetId) return
    const next = { ...drag, targetId }
    sortDragRef.current = next
    setSortDrag(next)
  }
  const endSortDrag = (event) => {
    const drag = sortDragRef.current
    if (!drag) return
    try {
      event.currentTarget.releasePointerCapture?.(event.pointerId)
    } catch {
      // Pointer capture may already be released by the browser.
    }
    sortDragRef.current = null
    setSortDrag(null)
    if (drag.targetId && drag.targetId !== drag.sourceId) {
      moveSettingsItem(drag.key, drag.sourceId, drag.targetId)
    }
  }
  const handleSortKeyDown = (key, item, event) => {
    if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return
    event.preventDefault()
    const items = settingItems(key)
    const index = items.findIndex(candidate => candidate.id === item.id)
    const target = items[index + (event.key === 'ArrowUp' ? -1 : 1)]
    if (target) moveSettingsItem(key, item.id, target.id)
  }
  const handleSubmitNewAllocationTemplate = async (event) => {
    setNewFormTouched(prev => ({ ...prev, allocation: { ...(prev.allocation || {}), submit: true } }))
    if (hasErrors(templateFormErrors)) {
      event.preventDefault()
      return
    }
    const saved = await submitAllocationTemplate(event)
    if (saved) setNewFormTouched(prev => ({ ...prev, allocation: {} }))
  }
  const handleSubmitNewIncomeTemplate = async (event) => {
    setNewFormTouched(prev => ({ ...prev, income: { ...(prev.income || {}), submit: true } }))
    if (hasErrors(incomeTemplateFormErrors)) {
      event.preventDefault()
      return
    }
    const saved = await submitIncomeTemplate(event)
    if (saved) setNewFormTouched(prev => ({ ...prev, income: {} }))
  }
  const handleSubmitNewCategory = async (event) => {
    setNewFormTouched(prev => ({ ...prev, category: { ...(prev.category || {}), submit: true } }))
    if (hasErrors(categoryFormErrors)) {
      event.preventDefault()
      return
    }
    const saved = await submitCategory(event)
    if (saved) setNewFormTouched(prev => ({ ...prev, category: {} }))
  }
  const updateCategoryPickerSelection = (next) => {
    if (categoryPicker?.kind === 'template-form') {
      setTemplateForm(prev => ({ ...prev, defaultCategoryIds: next }))
      return
    }
    if (categoryPicker?.templateId) {
      const template = (settings?.allocation_templates || []).find(item => item.id === categoryPicker.templateId)
      if (template) editSettingsItem('allocation_templates', template, { default_category_ids: next })
    }
  }
  return (
    <div className="wallet-settings-grid">
      <section className="wallet-panel wallet-settings-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Defaults</span>
            <strong>Allocation Templates</strong>
          </div>
        </div>
        <div className="wallet-template-list">
          {(settings?.allocation_templates || []).map(template => (
            <div
              key={template.id}
              className={sortRowClass('allocation_templates', template.id, 'wallet-template-row')}
              data-wallet-sort-key="allocation_templates"
              data-wallet-sort-id={template.id}
              onBlurCapture={event => handleSettingsRowBlur('allocation_templates', template, event)}
              onKeyDownCapture={event => handleSettingsRowKeyDown('allocation_templates', template, event)}
            >
              {(() => {
                const selectedCategoryIds = templateCategoryIds(template)
                const rowErrors = {
                  name: validateName(template.name, 'Allocation template name'),
                  amount: validateAmount(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount', { nonNegative: true }),
                }
                const displayErrors = rowAttempted('allocation_templates', template.id) ? rowErrors : {}
                return (
                  <>
                    <SettingsSortHandle
                      label={`Reorder ${template.name}`}
                      disabled={saving}
                      onPointerDown={event => beginSortDrag('allocation_templates', template, event)}
                      onPointerMove={updateSortDrag}
                      onPointerUp={endSortDrag}
                      onKeyDown={event => handleSortKeyDown('allocation_templates', template, event)}
                    />
                    <WalletField error={displayErrors.name}>
                      <input
                        value={template.name}
                        onChange={event => editSettingsItem('allocation_templates', template, { name: event.target.value })}
                        aria-label="Allocation template name"
                        {...withFieldError(displayErrors.name)}
                      />
                    </WalletField>
                    <WalletField error={displayErrors.amount}>
                      <input
                        value={template.amount_input ?? moneyInputValue(template.default_amount_cents)}
                        onChange={event => editSettingsItem('allocation_templates', template, { amount_input: event.target.value })}
                        inputMode="decimal"
                        aria-label="Allocation template amount"
                        {...withFieldError(displayErrors.amount)}
                      />
                    </WalletField>
                    <select
                      value={template.type}
                      onChange={event => editSettingsItem('allocation_templates', template, { type: event.target.value })}
                      aria-label="Allocation template type"
                    >
                      <option value="flexible">Flexible</option>
                      <option value="fixed">Fixed</option>
                      <option value="sinking_fund">Sinking Fund</option>
                      <option value="one_off">One-Off</option>
                    </select>
                    <AllocationCategoryButton
                      count={selectedCategoryIds.length}
                      onClick={() => setCategoryPicker({ templateId: template.id })}
                    />
                    <label className="wallet-settings-check">
                      <input
                        type="checkbox"
                        checked={!!template.carry_forward}
                        onChange={event => editSettingsItem('allocation_templates', template, { carry_forward: event.target.checked })}
                      />
                      <span>Carry</span>
                    </label>
                    <label className="wallet-settings-check">
                      <input
                        type="checkbox"
                        checked={!!template.active}
                        onChange={event => editSettingsItem('allocation_templates', template, { active: event.target.checked })}
                      />
                      <span>Active</span>
                    </label>
                    <button type="button" className="btn-ghost wallet-save-btn" onClick={() => handleSaveSettingsRow('allocation_templates', template, rowErrors, saveAllocationTemplate)} disabled={saving} title="Save template">
                      <Save size={12} />
                    </button>
                    <button type="button" className="btn-ghost wallet-danger-btn" onClick={() => deleteAllocationTemplate(template)} disabled={saving} title="Delete template">
                      <Trash2 size={12} />
                    </button>
                  </>
                )
              })()}
            </div>
          ))}
        </div>
        <form onSubmit={handleSubmitNewAllocationTemplate} className="wallet-template-form">
          <WalletField error={templateFormDisplayErrors.name}>
            <input
              value={templateForm.name}
              onChange={event => setTemplateForm(prev => ({ ...prev, name: event.target.value }))}
              onBlur={() => setNewTouched('allocation', 'name')}
              placeholder="Allocation template"
              {...withFieldError(templateFormDisplayErrors.name)}
            />
          </WalletField>
          <WalletField error={templateFormDisplayErrors.amount}>
            <input
              value={templateForm.amount}
              onChange={event => setTemplateForm(prev => ({ ...prev, amount: event.target.value }))}
              onBlur={() => setNewTouched('allocation', 'amount')}
              placeholder="Amount"
              inputMode="decimal"
              {...withFieldError(templateFormDisplayErrors.amount)}
            />
          </WalletField>
          <select
            value={templateForm.type}
            onChange={event => setTemplateForm(prev => ({ ...prev, type: event.target.value }))}
          >
            <option value="flexible">Flexible</option>
            <option value="fixed">Fixed</option>
            <option value="sinking_fund">Sinking Fund</option>
            <option value="one_off">One-Off</option>
          </select>
          <label className="wallet-settings-check">
            <input
              type="checkbox"
              checked={templateForm.carryForward}
              onChange={event => setTemplateForm(prev => ({ ...prev, carryForward: event.target.checked }))}
            />
            <span>Carry</span>
          </label>
          <AllocationCategoryButton
            count={(templateForm.defaultCategoryIds || []).length}
            onClick={() => setCategoryPicker({ kind: 'template-form' })}
          />
          <button type="submit" className="btn-ghost" disabled={saving || hasErrors(templateFormErrors)}>
            <Plus size={13} />
            Add
          </button>
        </form>
      </section>

      <section className="wallet-panel wallet-settings-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Defaults</span>
            <strong>Income Templates</strong>
          </div>
        </div>
        <div className="wallet-template-list">
          {(settings?.income_templates || []).map(template => (
            <div
              key={template.id}
              className={sortRowClass('income_templates', template.id, 'wallet-income-template-row')}
              data-wallet-sort-key="income_templates"
              data-wallet-sort-id={template.id}
              onBlurCapture={event => handleSettingsRowBlur('income_templates', template, event)}
              onKeyDownCapture={event => handleSettingsRowKeyDown('income_templates', template, event)}
            >
              {(() => {
                const rowErrors = {
                  name: validateName(template.name, 'Income template name'),
                  amount: validateAmount(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount', { nonNegative: true }),
                  defaultDay: validateInteger(template.default_day_input ?? template.default_day ?? '', 'Default day', { min: 1, max: 31 }),
                }
                const displayErrors = rowAttempted('income_templates', template.id) ? rowErrors : {}
                return (
                  <>
                    <SettingsSortHandle
                      label={`Reorder ${template.name}`}
                      disabled={saving}
                      onPointerDown={event => beginSortDrag('income_templates', template, event)}
                      onPointerMove={updateSortDrag}
                      onPointerUp={endSortDrag}
                      onKeyDown={event => handleSortKeyDown('income_templates', template, event)}
                    />
                    <WalletField error={displayErrors.name}>
                      <input
                        value={template.name}
                        onChange={event => editSettingsItem('income_templates', template, { name: event.target.value })}
                        aria-label="Income template name"
                        {...withFieldError(displayErrors.name)}
                      />
                    </WalletField>
                    <WalletField error={displayErrors.amount}>
                      <input
                        value={template.amount_input ?? moneyInputValue(template.default_amount_cents)}
                        onChange={event => editSettingsItem('income_templates', template, { amount_input: event.target.value })}
                        inputMode="decimal"
                        aria-label="Income template amount"
                        {...withFieldError(displayErrors.amount)}
                      />
                    </WalletField>
                    <WalletField error={displayErrors.defaultDay}>
                      <input
                        type="number"
                        min="1"
                        max="31"
                        value={template.default_day_input ?? template.default_day ?? ''}
                        onChange={event => editSettingsItem('income_templates', template, { default_day_input: event.target.value })}
                        placeholder="Day"
                        aria-label="Income template default day"
                        {...withFieldError(displayErrors.defaultDay)}
                      />
                    </WalletField>
                    <label className="wallet-settings-check">
                      <input
                        type="checkbox"
                        checked={!!template.active}
                        onChange={event => editSettingsItem('income_templates', template, { active: event.target.checked })}
                      />
                      <span>Active</span>
                    </label>
                    <button type="button" className="btn-ghost wallet-save-btn" onClick={() => handleSaveSettingsRow('income_templates', template, rowErrors, saveIncomeTemplate)} disabled={saving} title="Save template">
                      <Save size={12} />
                    </button>
                    <button type="button" className="btn-ghost wallet-danger-btn" onClick={() => deleteIncomeTemplate(template)} disabled={saving} title="Delete template">
                      <Trash2 size={12} />
                    </button>
                  </>
                )
              })()}
            </div>
          ))}
        </div>
        <form onSubmit={handleSubmitNewIncomeTemplate} className="wallet-income-template-form">
          <WalletField error={incomeTemplateFormDisplayErrors.name}>
            <input
              value={incomeTemplateForm.name}
              onChange={event => setIncomeTemplateForm(prev => ({ ...prev, name: event.target.value }))}
              onBlur={() => setNewTouched('income', 'name')}
              placeholder="Income template"
              {...withFieldError(incomeTemplateFormDisplayErrors.name)}
            />
          </WalletField>
          <WalletField error={incomeTemplateFormDisplayErrors.amount}>
            <input
              value={incomeTemplateForm.amount}
              onChange={event => setIncomeTemplateForm(prev => ({ ...prev, amount: event.target.value }))}
              onBlur={() => setNewTouched('income', 'amount')}
              placeholder="Amount"
              inputMode="decimal"
              {...withFieldError(incomeTemplateFormDisplayErrors.amount)}
            />
          </WalletField>
          <WalletField error={incomeTemplateFormDisplayErrors.defaultDay}>
            <input
              type="number"
              min="1"
              max="31"
              value={incomeTemplateForm.defaultDay}
              onChange={event => setIncomeTemplateForm(prev => ({ ...prev, defaultDay: event.target.value }))}
              onBlur={() => setNewTouched('income', 'defaultDay')}
              placeholder="Day"
              {...withFieldError(incomeTemplateFormDisplayErrors.defaultDay)}
            />
          </WalletField>
          <button type="submit" className="btn-ghost" disabled={saving || hasErrors(incomeTemplateFormErrors)}>
            <Plus size={13} />
            Add
          </button>
        </form>
      </section>

      <section className="wallet-panel wallet-settings-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Defaults</span>
            <strong>Categories</strong>
          </div>
        </div>
        <div className="wallet-template-list">
          {(settings?.categories || []).map(category => {
            const protectedCategory = category.system_key === 'unsorted'
            const rowErrors = { name: validateName(category.name, 'Category name') }
            const displayErrors = rowAttempted('categories', category.id) ? rowErrors : {}
            return (
              <div
                key={category.id}
                className={sortRowClass('categories', category.id, 'wallet-category-row')}
                data-wallet-sort-key="categories"
                data-wallet-sort-id={category.id}
                onBlurCapture={event => handleSettingsRowBlur('categories', category, event)}
                onKeyDownCapture={event => handleSettingsRowKeyDown('categories', category, event)}
              >
                <SettingsSortHandle
                  label={`Reorder ${category.name}`}
                  disabled={saving}
                  onPointerDown={event => beginSortDrag('categories', category, event)}
                  onPointerMove={updateSortDrag}
                  onPointerUp={endSortDrag}
                  onKeyDown={event => handleSortKeyDown('categories', category, event)}
                />
                <WalletField error={displayErrors.name}>
                  <input
                    value={category.name}
                    onChange={event => editSettingsItem('categories', category, { name: event.target.value })}
                    aria-label="Category name"
                    {...withFieldError(displayErrors.name)}
                  />
                </WalletField>
                <label className="wallet-settings-check">
                  <input
                    type="checkbox"
                    checked={!!category.active}
                    disabled={protectedCategory}
                    onChange={event => editSettingsItem('categories', category, { active: event.target.checked })}
                  />
                  <span>Active</span>
                </label>
                <button type="button" className="btn-ghost wallet-save-btn" onClick={() => handleSaveSettingsRow('categories', category, rowErrors, saveCategory)} disabled={saving} title="Save category">
                  <Save size={12} />
                </button>
                <button type="button" className="btn-ghost wallet-danger-btn" onClick={() => deleteCategory(category)} disabled={saving || protectedCategory} title="Delete category">
                  <Trash2 size={12} />
                </button>
              </div>
            )
          })}
        </div>
        <form onSubmit={handleSubmitNewCategory} className="wallet-category-form">
          <WalletField error={categoryFormDisplayErrors.name}>
            <input
              value={categoryForm.name}
              onChange={event => setCategoryForm({ name: event.target.value })}
              onBlur={() => setNewTouched('category', 'name')}
              placeholder="Category name"
              {...withFieldError(categoryFormDisplayErrors.name)}
            />
          </WalletField>
          <button type="submit" className="btn-ghost" disabled={saving || hasErrors(categoryFormErrors)}>
            <Plus size={13} />
            Add
          </button>
        </form>
      </section>

      {categoryPicker && (
        <AllocationCategoryModal
          title={categoryPickerTitle}
          categories={activeCategories}
          selectedIds={categoryPickerSelectedIds}
          onChange={updateCategoryPickerSelection}
          onClose={() => setCategoryPicker(null)}
        />
      )}
    </div>
  )
}

function MonthPreviewPanel({
  preview,
  saving,
  updatePreviewIncome,
  updatePreviewAllocation,
  confirmCreateMonth,
  cancelPreview,
  previewInvalid,
  previewErrors,
}) {
  return (
    <section className="wallet-panel wallet-preview-panel">
      <div className="wallet-panel-header">
        <div>
          <span className="wallet-section-label">Create Month</span>
          <strong>Preview {formatMonthLabel(preview.month.month)}</strong>
        </div>
        <div className="wallet-filter-actions">
          <button type="button" className="btn-ghost" onClick={cancelPreview} disabled={saving}>Cancel</button>
          <button type="button" className="btn-primary" onClick={confirmCreateMonth} disabled={saving || previewInvalid}>
            <Save size={14} />
            Create Month
          </button>
        </div>
      </div>
      <div className="wallet-preview-summary">
        <SummaryMetric label="Opening" value={preview.month.opening_balance_cents} />
        <SummaryMetric label="Wallet" value={preview.month.wallet_balance_cents} />
        <SummaryMetric label="Income Rows" value={(preview.income_items || []).reduce((sum, item) => sum + (item.amount_cents || 0), 0)} tone="good" />
        <SummaryMetric label="Allocation Rows" value={(preview.allocations || []).reduce((sum, item) => sum + (item.budgeted_cents || 0), 0)} />
      </div>
      <div className="wallet-preview-grid">
        <div>
          <div className="wallet-subsection-title">Income</div>
          <div className="wallet-preview-list">
            {(preview.income_items || []).length === 0 ? (
              <div className="wallet-empty-row">No generated income rows.</div>
            ) : preview.income_items.map((item, index) => {
              const errors = previewErrors.income[index] || {}
              return (
                <div key={item.id || index} className="wallet-preview-income-row">
                  <WalletField error={errors.name}>
                    <input
                      value={item.name}
                      onChange={event => updatePreviewIncome(index, { name: event.target.value })}
                      aria-label="Income name"
                      {...withFieldError(errors.name)}
                    />
                  </WalletField>
                  <WalletField error={errors.amount}>
                    <input
                      value={item.amount_input ?? moneyInputValue(item.amount_cents)}
                      onChange={event => updatePreviewIncome(index, { amount_input: event.target.value })}
                      inputMode="decimal"
                      aria-label="Income amount"
                      {...withFieldError(errors.amount)}
                    />
                  </WalletField>
                  <WalletField error={errors.receivedDate}>
                    <input
                      type="date"
                      value={item.received_date || ''}
                      onChange={event => updatePreviewIncome(index, { received_date: event.target.value || null })}
                      aria-label="Income received date"
                      {...withFieldError(errors.receivedDate)}
                    />
                  </WalletField>
                  <WalletField error={errors.appliesToMonth}>
                    <input
                      type="month"
                      value={item.applies_to_month}
                      onChange={event => updatePreviewIncome(index, { applies_to_month: event.target.value })}
                      aria-label="Income applies to month"
                      {...withFieldError(errors.appliesToMonth)}
                    />
                  </WalletField>
                  <input
                    value={item.notes || ''}
                    onChange={event => updatePreviewIncome(index, { notes: event.target.value })}
                    placeholder="Notes"
                    aria-label="Income notes"
                  />
                </div>
              )
            })}
          </div>
        </div>
        <div>
          <div className="wallet-subsection-title">Allocations</div>
          <div className="wallet-preview-list">
            {(preview.allocations || []).length === 0 ? (
              <div className="wallet-empty-row">No generated allocations.</div>
            ) : preview.allocations.map((allocation, index) => {
              const errors = previewErrors.allocations[index] || {}
              return (
                <div key={allocation.id || index} className="wallet-preview-allocation-row">
                  <WalletField error={errors.name}>
                    <input
                      value={allocation.name}
                      onChange={event => updatePreviewAllocation(index, { name: event.target.value })}
                      aria-label="Allocation name"
                      {...withFieldError(errors.name)}
                    />
                  </WalletField>
                  <WalletField error={errors.amount}>
                    <input
                      value={allocation.amount_input ?? moneyInputValue(allocation.budgeted_cents)}
                      onChange={event => updatePreviewAllocation(index, { amount_input: event.target.value })}
                      inputMode="decimal"
                      aria-label="Allocation amount"
                      {...withFieldError(errors.amount)}
                    />
                  </WalletField>
                  <select
                    value={allocation.type}
                    onChange={event => updatePreviewAllocation(index, { type: event.target.value })}
                    aria-label="Allocation type"
                  >
                    <option value="flexible">Flexible</option>
                    <option value="fixed">Fixed</option>
                    <option value="sinking_fund">Sinking Fund</option>
                    <option value="one_off">One-Off</option>
                  </select>
                  <label className="wallet-settings-check">
                    <input
                      type="checkbox"
                      checked={!!allocation.carry_forward}
                      onChange={event => updatePreviewAllocation(index, { carry_forward: event.target.checked })}
                    />
                    <span>Carry</span>
                  </label>
                  <label className="wallet-settings-check">
                    <input
                      type="checkbox"
                      checked={allocation.active !== false}
                      onChange={event => updatePreviewAllocation(index, { active: event.target.checked })}
                    />
                    <span>Active</span>
                  </label>
                </div>
              )
            })}
          </div>
        </div>
      </div>
    </section>
  )
}

function WalletReviewView({
  summary,
  saving,
  monthClosed,
  reviewFilters,
  setReviewFilters,
  reviewTransactions,
  updateReviewTransaction,
  saveReviewTransaction,
  reloadReview,
  detailAllocationId,
  setDetailAllocationId,
  allocationDetail,
  allocationDetailForm,
  setAllocationDetailForm,
  saveAllocationDetail,
  allocationDetailTransactionForm,
  setAllocationDetailTransactionForm,
  allocationDetailTransactionTouched,
  touchAllocationDetailTransaction,
  submitAllocationDetailTransaction,
  openSplitModal,
  openSplitDetail,
  deleteTransaction,
}) {
  const allocations = summary?.allocations || []
  const categories = summary?.categories || []
  const detailCategoryOptions = orderedCategoriesForAllocation(allocationDetail?.allocation, categories)
  const detailEditErrors = allocationDetailForm ? {
    name: validateName(allocationDetailForm.name, 'Allocation name'),
    amount: validateAmount(allocationDetailForm.amount, 'Allocation budget', { nonNegative: true }),
  } : {}
  const detailTransactionErrors = {
    amount: validateAmount(allocationDetailTransactionForm.amount, 'Transaction amount', { required: true, positive: true }),
    date: validateDate(allocationDetailTransactionForm.date, 'Transaction date'),
    category: allocationDetailTransactionForm.categoryId ? '' : 'Category is required',
  }
  const detailTransactionDisplayErrors = {
    amount: visibleFieldError(detailTransactionErrors, allocationDetailTransactionTouched, 'amount'),
    date: visibleFieldError(detailTransactionErrors, allocationDetailTransactionTouched, 'date'),
    category: visibleFieldError(detailTransactionErrors, allocationDetailTransactionTouched, 'category'),
  }
  return (
    <div className="wallet-review-grid">
      <section className="wallet-panel wallet-review-main">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Review</span>
            <strong>Cleanup Queue</strong>
          </div>
          <div className="wallet-filter-actions">
            <label className="wallet-settings-check">
              <input
                type="checkbox"
                checked={reviewFilters.cleanup}
                onChange={event => setReviewFilters(prev => ({ ...prev, cleanup: event.target.checked }))}
              />
              <span>Cleanup</span>
            </label>
            <label className="wallet-settings-check">
              <input
                type="checkbox"
                checked={reviewFilters.unsorted}
                onChange={event => setReviewFilters(prev => ({ ...prev, unsorted: event.target.checked }))}
              />
              <span>Unsorted</span>
            </label>
            <label className="wallet-settings-check">
              <input
                type="checkbox"
                checked={reviewFilters.rounded}
                onChange={event => setReviewFilters(prev => ({ ...prev, rounded: event.target.checked }))}
              />
              <span>Rounded</span>
            </label>
            <button type="button" className="btn-ghost" onClick={reloadReview} disabled={saving}>
              <RefreshCw size={13} />
              Reload
            </button>
          </div>
        </div>
        <div className="wallet-review-table">
          {reviewTransactions.length === 0 ? (
            <div className="wallet-empty-row">No transactions need review.</div>
          ) : reviewTransactions.map(transaction => {
            const rowErrors = {
              date: validateDate(transaction.date, 'Transaction date'),
              amount: isSplitChild(transaction) ? '' : validateAmount(transaction.amount_input ?? moneyInputValue(transaction.amount_cents), 'Transaction amount', { required: true, positive: true }),
              allocation: transaction.allocation_id ? '' : 'Allocation is required',
              category: transaction.category_id ? '' : 'Category is required',
            }
            const label = splitRoleLabel(transaction)
            return (
              <div key={transaction.id} className="wallet-review-row">
                <WalletField error={rowErrors.date}>
                  <input
                    type="date"
                    value={transaction.date}
                    onChange={event => updateReviewTransaction(transaction.id, { date: event.target.value })}
                    aria-label="Transaction date"
                    {...withFieldError(rowErrors.date)}
                  />
                </WalletField>
                <WalletField error={rowErrors.amount}>
                  <input
                    value={transaction.amount_input ?? moneyInputValue(transaction.amount_cents)}
                    onChange={event => updateReviewTransaction(transaction.id, { amount_input: event.target.value })}
                    inputMode="decimal"
                    aria-label="Transaction amount"
                    disabled={isSplitChild(transaction)}
                    {...withFieldError(rowErrors.amount)}
                  />
                </WalletField>
                <WalletField error={rowErrors.allocation}>
                  <select
                    value={transaction.allocation_id}
                    onChange={event => updateReviewTransaction(transaction.id, { allocation_id: event.target.value })}
                    aria-label="Transaction allocation"
                    {...withFieldError(rowErrors.allocation)}
                  >
                    {allocations.map(allocation => (
                      <option key={allocation.id} value={allocation.id}>{allocation.name}</option>
                    ))}
                  </select>
                </WalletField>
                <WalletField error={rowErrors.category}>
                  <select
                    value={transaction.category_id}
                    onChange={event => updateReviewTransaction(transaction.id, { category_id: event.target.value })}
                    aria-label="Transaction category"
                    {...withFieldError(rowErrors.category)}
                  >
                    {categories.map(category => (
                      <option key={category.id} value={category.id}>{category.name}</option>
                    ))}
                  </select>
                </WalletField>
                <div className="wallet-review-note-cell">
                  <input
                    value={transaction.note_input ?? transaction.note ?? ''}
                    onChange={event => updateReviewTransaction(transaction.id, { note_input: event.target.value })}
                    placeholder="Note"
                    aria-label="Transaction note"
                  />
                  {label && <span className="wallet-split-badge">{label}</span>}
                </div>
                <label className="wallet-settings-check">
                  <input
                    type="checkbox"
                    checked={!!transaction.rounded}
                    onChange={event => updateReviewTransaction(transaction.id, { rounded: event.target.checked })}
                  />
                  <span>Rounded</span>
                </label>
                <div className="wallet-review-actions">
                  <button type="button" className="btn-ghost wallet-review-action-btn" onClick={() => saveReviewTransaction(transaction)} disabled={saving || monthClosed || hasErrors(rowErrors)} title="Save transaction" aria-label="Save transaction">
                    <Save size={14} />
                  </button>
                  <button
                    type="button"
                    className="btn-ghost wallet-review-action-btn"
                    onClick={() => isSplitParent(transaction) ? openSplitDetail(transaction) : openSplitModal(transaction)}
                    disabled={saving || isSplitChild(transaction) || (!isSplitParent(transaction) && monthClosed)}
                    title={isSplitParent(transaction) ? 'View split group' : 'Split transaction'}
                    aria-label={isSplitParent(transaction) ? 'View split group' : 'Split transaction'}
                  >
                    <Split size={14} />
                  </button>
                  <button
                    type="button"
                    className="btn-ghost wallet-danger-btn wallet-review-action-btn"
                    onClick={() => deleteTransaction(transaction)}
                    disabled={saving || monthClosed || isSplitChild(transaction)}
                    title={isSplitChild(transaction) ? 'Split child delete is blocked' : 'Delete transaction'}
                    aria-label={isSplitChild(transaction) ? 'Split child delete is blocked' : 'Delete transaction'}
                  >
                    <Trash2 size={14} />
                  </button>
                </div>
              </div>
            )
          })}
        </div>
      </section>

      <section className="wallet-panel wallet-review-detail">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Allocation Detail</span>
            <strong>{allocationDetail?.allocation?.name || 'Select allocation'}</strong>
          </div>
          <select value={detailAllocationId} onChange={event => setDetailAllocationId(event.target.value)}>
            {allocations.map(allocation => (
              <option key={allocation.id} value={allocation.id}>{allocation.name}</option>
            ))}
          </select>
        </div>
        {allocationDetail ? (
          <>
            <div className="wallet-detail-summary">
              <SummaryMetric label="Budgeted" value={allocationDetail.allocation.budgeted_cents} />
              <SummaryMetric label="Spent" value={allocationDetail.allocation.spent_cents} />
              <SummaryMetric
                label="Remaining"
                value={allocationDetail.allocation.remaining_cents}
                tone={allocationDetail.allocation.remaining_cents < 0 ? 'bad' : 'good'}
              />
            </div>
            <div className="wallet-breakdown-list">
              {allocationDetail.category_breakdown.length === 0 ? (
                <div className="wallet-empty-row">No category spend yet.</div>
              ) : allocationDetail.category_breakdown.map(row => (
                <div key={row.category_id} className="wallet-breakdown-row">
                  <div>
                    <strong>{row.category_name}</strong>
                    <span>{row.count} transaction{row.count === 1 ? '' : 's'}</span>
                  </div>
                  <strong>{formatMoney(row.amount_cents)}</strong>
                </div>
              ))}
            </div>
            {allocationDetailForm && (
              <form onSubmit={saveAllocationDetail} className="wallet-allocation-detail-form">
                <WalletField error={detailEditErrors.name}>
                  <input
                    value={allocationDetailForm.name}
                    onChange={event => setAllocationDetailForm(prev => ({ ...prev, name: event.target.value }))}
                    placeholder="Allocation name"
                    {...withFieldError(detailEditErrors.name)}
                  />
                </WalletField>
                <WalletField error={detailEditErrors.amount}>
                  <input
                    value={allocationDetailForm.amount}
                    onChange={event => setAllocationDetailForm(prev => ({ ...prev, amount: event.target.value }))}
                    inputMode="decimal"
                    placeholder="Budget"
                    {...withFieldError(detailEditErrors.amount)}
                  />
                </WalletField>
                <select value={allocationDetailForm.type} onChange={event => setAllocationDetailForm(prev => ({ ...prev, type: event.target.value }))}>
                  <option value="flexible">Flexible</option>
                  <option value="fixed">Fixed</option>
                  <option value="sinking_fund">Sinking Fund</option>
                  <option value="one_off">One-Off</option>
                </select>
                <label className="wallet-settings-check">
                  <input
                    type="checkbox"
                    checked={!!allocationDetailForm.carryForward}
                    onChange={event => setAllocationDetailForm(prev => ({ ...prev, carryForward: event.target.checked }))}
                  />
                  <span>Carry</span>
                </label>
                <label className="wallet-settings-check">
                  <input
                    type="checkbox"
                    checked={!!allocationDetailForm.active}
                    onChange={event => setAllocationDetailForm(prev => ({ ...prev, active: event.target.checked }))}
                  />
                  <span>Active</span>
                </label>
                <button type="submit" className="btn-ghost" disabled={saving || monthClosed || hasErrors(detailEditErrors)}>
                  <Save size={13} />
                  Save
                </button>
              </form>
            )}
            <form onSubmit={submitAllocationDetailTransaction} className="wallet-detail-transaction-form">
              <WalletField error={detailTransactionDisplayErrors.amount}>
                <input
                  value={allocationDetailTransactionForm.amount}
                  onChange={event => setAllocationDetailTransactionForm(prev => ({ ...prev, amount: event.target.value }))}
                  onBlur={() => touchAllocationDetailTransaction('amount')}
                  placeholder="Amount"
                  inputMode="decimal"
                  disabled={monthClosed}
                  {...withFieldError(detailTransactionDisplayErrors.amount)}
                />
              </WalletField>
              <WalletField error={detailTransactionDisplayErrors.category}>
                <select
                  value={allocationDetailTransactionForm.categoryId}
                  onChange={event => setAllocationDetailTransactionForm(prev => ({ ...prev, categoryId: event.target.value }))}
                  onBlur={() => touchAllocationDetailTransaction('category')}
                  disabled={monthClosed}
                  {...withFieldError(detailTransactionDisplayErrors.category)}
                >
                  {detailCategoryOptions.map(category => (
                    <option key={category.id} value={category.id}>{category.name}</option>
                  ))}
                </select>
              </WalletField>
              <WalletField error={detailTransactionDisplayErrors.date}>
                <input
                  type="date"
                  value={allocationDetailTransactionForm.date}
                  onChange={event => setAllocationDetailTransactionForm(prev => ({ ...prev, date: event.target.value }))}
                  onBlur={() => touchAllocationDetailTransaction('date')}
                  disabled={monthClosed}
                  {...withFieldError(detailTransactionDisplayErrors.date)}
                />
              </WalletField>
              <input
                value={allocationDetailTransactionForm.note}
                onChange={event => setAllocationDetailTransactionForm(prev => ({ ...prev, note: event.target.value }))}
                placeholder="Note"
                disabled={monthClosed}
              />
              <label className="wallet-settings-check">
                <input
                  type="checkbox"
                  checked={!!allocationDetailTransactionForm.rounded}
                  onChange={event => setAllocationDetailTransactionForm(prev => ({ ...prev, rounded: event.target.checked }))}
                  disabled={monthClosed}
                />
                <span>Rounded</span>
              </label>
              <button type="submit" className="btn-primary" disabled={saving || monthClosed || hasErrors(detailTransactionErrors)}>
                <Plus size={14} />
                Add
              </button>
            </form>
          </>
        ) : (
          <div className="wallet-empty-row">No allocation selected.</div>
        )}
      </section>
    </div>
  )
}

function WalletReportsView({
  monthKey,
  setMonthKey,
  months,
  summary,
  saving,
  reportFrom,
  setReportFrom,
  reportTo,
  setReportTo,
  reportAllocationFilter,
  setReportAllocationFilter,
  monthlyReport,
  allocationReport,
  categoryReport,
  reviewReport,
  reloadReports,
  closeMonth,
  reopenMonth,
}) {
  return (
    <div className="wallet-reports-grid">
      <section className="wallet-panel wallet-reports-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Reports</span>
            <strong>Monthly Summary</strong>
          </div>
          <div className="wallet-report-filters">
            <input type="month" value={reportFrom} onChange={event => setReportFrom(event.target.value)} aria-label="Report from month" />
            <input type="month" value={reportTo} onChange={event => setReportTo(event.target.value)} aria-label="Report to month" />
            <button type="button" className="btn-ghost" onClick={reloadReports} disabled={saving}>
              <RefreshCw size={13} />
              Load
            </button>
          </div>
        </div>
        <div className="wallet-report-table">
          {monthlyReport.length === 0 ? (
            <div className="wallet-empty-row">No monthly report rows.</div>
          ) : monthlyReport.map(row => (
            <button key={row.month} type="button" className="wallet-report-row" onClick={() => setMonthKey(row.month)}>
              <span>{row.month}</span>
              <span>{row.status}</span>
              <strong>{formatMoney(row.wallet_balance_cents)}</strong>
              <span>{formatMoney(row.spending_total_cents)} spent</span>
              <span>{formatMoney(row.available_balance_cents)} available</span>
              <span className={row.variance_cents === 0 ? 'is-good' : 'is-warn'}>{formatMoney(row.variance_cents)} variance</span>
            </button>
          ))}
        </div>
      </section>

      <section className="wallet-panel wallet-close-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Month Close</span>
            <strong>{summary ? formatMonthLabel(monthKey) : 'No month'}</strong>
          </div>
          {summary?.month?.status === 'closed' ? (
            <button type="button" className="btn-ghost" onClick={reopenMonth} disabled={saving}>
              <RefreshCw size={13} />
              Reopen
            </button>
          ) : (
            <button type="button" className="btn-primary" onClick={closeMonth} disabled={saving || !summary}>
              <CheckCircle2 size={14} />
              Close Month
            </button>
          )}
        </div>
        {summary ? (
          <div className="wallet-close-grid">
            <SummaryMetric label="Opening" value={summary.month.opening_balance_cents} />
            <SummaryMetric label="Income" value={summary.income_total_cents} tone="good" />
            <SummaryMetric label="Spending" value={summary.spending_total_cents} />
            <SummaryMetric label="Expected" value={summary.expected_balance_cents} />
            <SummaryMetric label="Final Wallet" value={summary.wallet_balance_cents} />
            <SummaryMetric label="Variance" value={summary.variance_cents} tone={summary.variance_cents === 0 ? 'good' : 'warn'} />
            <SummaryMetric label="Reserved" value={summary.total_reserved_cents} />
            <SummaryMetric label="Available" value={summary.available_balance_cents} tone={summary.available_balance_cents < 0 ? 'bad' : 'good'} />
          </div>
        ) : (
          <div className="wallet-empty-row">Create the month before closing it.</div>
        )}
        {reviewReport && (
          <div className="wallet-review-report">
            <span>{reviewReport.review_counts.unsorted_count} unsorted</span>
            <span>{reviewReport.review_counts.rounded_count} rounded</span>
            <span>{formatMoney(reviewReport.adjustment_total_cents)} adjustments</span>
          </div>
        )}
        {months.length > 0 && (
          <div className="wallet-month-hints">
            {months.slice(0, 8).map(month => (
              <button key={month.id} type="button" className="btn-ghost" onClick={() => setMonthKey(month.month)}>
                {month.month}
              </button>
            ))}
          </div>
        )}
      </section>

      <section className="wallet-panel wallet-reports-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Reports</span>
            <strong>Allocations</strong>
          </div>
        </div>
        <div className="wallet-report-table">
          {allocationReport.length === 0 ? (
            <div className="wallet-empty-row">No allocation report rows.</div>
          ) : allocationReport.map(row => (
            <div key={row.id} className="wallet-report-row">
              <span>{row.name}</span>
              <strong>{formatMoney(row.budgeted_cents)}</strong>
              <span>{formatMoney(row.spent_cents)} spent</span>
              <span>{formatMoney(row.remaining_cents)} left</span>
            </div>
          ))}
        </div>
      </section>

      <section className="wallet-panel wallet-reports-panel">
        <div className="wallet-panel-header">
          <div>
            <span className="wallet-section-label">Reports</span>
            <strong>Categories</strong>
          </div>
          <div className="wallet-report-filters">
            <select
              value={reportAllocationFilter}
              onChange={event => setReportAllocationFilter(event.target.value)}
              aria-label="Category report allocation filter"
            >
              <option value="">All allocations</option>
              {(summary?.allocations || []).map(allocation => (
                <option key={allocation.id} value={allocation.id}>{allocation.name}</option>
              ))}
            </select>
            <button type="button" className="btn-ghost" onClick={reloadReports} disabled={saving}>
              <RefreshCw size={13} />
              Load
            </button>
          </div>
        </div>
        <div className="wallet-report-table">
          {categoryReport.length === 0 ? (
            <div className="wallet-empty-row">No category report rows.</div>
          ) : categoryReport.map(row => (
            <div key={row.category_id} className="wallet-report-row">
              <span>{row.category_name}</span>
              <strong>{formatMoney(row.amount_cents)}</strong>
              <span>{row.count} tx</span>
              <span>{row.percent_of_spend.toFixed(1)}%</span>
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

function SplitTransactionModal({ transaction, summary, rows, setRows, saving, onClose, onSubmit }) {
  const allocations = summary?.allocations || []
  const categories = summary?.categories || []
  let splitTotal = 0
  let invalidAmount = false
  const rowErrors = rows.map(row => ({
    amount: validateAmount(row.amount, 'Split amount', { required: true, positive: true }),
    allocation: row.allocationId ? '' : 'Allocation is required',
    category: row.categoryId ? '' : 'Category is required',
  }))
  for (const row of rows) {
    try {
      splitTotal += parseCents(row.amount, 'Split amount')
    } catch {
      invalidAmount = true
    }
  }
  const difference = transaction ? transaction.amount_cents - splitTotal : 0
  return (
    <WalletModal title="Split Transaction" onClose={onClose}>
      <form onSubmit={onSubmit} className="wallet-split-form">
        <div className="wallet-split-summary">
          <span>{transaction.date}</span>
          <strong>{formatMoney(transaction.amount_cents)}</strong>
          <span>{transaction.allocation_name} / {transaction.category_name}</span>
        </div>
        <div className="wallet-split-rows">
          {rows.map((row, index) => (
            <div key={index} className="wallet-split-row">
              <WalletField error={rowErrors[index].amount}>
                <input
                  value={row.amount}
                  onChange={event => setRows(prev => prev.map((item, i) => i === index ? { ...item, amount: event.target.value } : item))}
                  placeholder="Amount"
                  inputMode="decimal"
                  {...withFieldError(rowErrors[index].amount)}
                />
              </WalletField>
              <WalletField error={rowErrors[index].allocation}>
                <select
                  value={row.allocationId}
                  onChange={event => setRows(prev => prev.map((item, i) => i === index ? { ...item, allocationId: event.target.value } : item))}
                  {...withFieldError(rowErrors[index].allocation)}
                >
                  {allocations.map(allocation => (
                    <option key={allocation.id} value={allocation.id}>{allocation.name}</option>
                  ))}
                </select>
              </WalletField>
              <WalletField error={rowErrors[index].category}>
                <select
                  value={row.categoryId}
                  onChange={event => setRows(prev => prev.map((item, i) => i === index ? { ...item, categoryId: event.target.value } : item))}
                  {...withFieldError(rowErrors[index].category)}
                >
                  {categories.map(category => (
                    <option key={category.id} value={category.id}>{category.name}</option>
                  ))}
                </select>
              </WalletField>
              <input
                value={row.note}
                onChange={event => setRows(prev => prev.map((item, i) => i === index ? { ...item, note: event.target.value } : item))}
                placeholder="Note"
              />
              <button
                type="button"
                className="btn-ghost wallet-danger-btn"
                onClick={() => setRows(prev => prev.filter((_, i) => i !== index))}
                disabled={rows.length <= 2}
                title="Remove split row"
              >
                <Trash2 size={12} />
              </button>
            </div>
          ))}
        </div>
        <div className="wallet-split-footer">
          <button
            type="button"
            className="btn-ghost"
            onClick={() => setRows(prev => [...prev, {
              amount: '',
              allocationId: transaction.allocation_id,
              categoryId: transaction.category_id,
              note: '',
            }])}
          >
            <Plus size={13} />
            Add Row
          </button>
          <span className={difference === 0 && !invalidAmount ? 'is-good' : 'is-warn'}>
            Difference {formatMoney(difference)}
          </span>
          <button type="submit" className="btn-primary" disabled={saving || invalidAmount || rowErrors.some(hasErrors) || difference !== 0}>
            <Split size={14} />
            Save Split
          </button>
        </div>
      </form>
    </WalletModal>
  )
}

function SplitGroupModal({ detail, onClose }) {
  if (!detail) return null
  return (
    <WalletModal title="Split Group" onClose={onClose}>
      <div className="wallet-split-detail">
        <div className="wallet-split-summary">
          <span>{detail.parent.date}</span>
          <strong>{formatMoney(detail.parent.amount_cents)}</strong>
          <span>{detail.parent.allocation_name} / {detail.parent.category_name}</span>
        </div>
        <div className="wallet-transaction-list">
          {(detail.children || []).map(child => (
            <div key={child.id} className="wallet-transaction-row wallet-split-child-row">
              <span className="wallet-transaction-date">{child.date}</span>
              <div>
                <strong>{child.note || child.category_name}</strong>
                <span>{child.allocation_name} / {child.category_name}</span>
              </div>
              <strong>{formatMoney(child.amount_cents)}</strong>
              <span className="wallet-split-badge">Split child</span>
            </div>
          ))}
        </div>
      </div>
    </WalletModal>
  )
}

function MonthBookModal({
  rows,
  currentMonth,
  saving,
  summaryMonth,
  editForm,
  setEditForm,
  onClose,
  onView,
  onReports,
  onSummary,
  onEdit,
  onSaveEdit,
  onDelete,
  onReopen,
}) {
  const selectedRow = rows.find(row => row.month === summaryMonth) || rows[0]
  const editErrors = editForm && editForm.status !== 'closed' ? {
    opening: validateAmount(editForm.opening, 'Opening balance', { required: true, nonNegative: true }),
    wallet: validateAmount(editForm.wallet, 'Wallet balance', { required: true, nonNegative: true }),
  } : {}

  return (
    <WalletModal title="Wallet Month Book" onClose={onClose} wide>
      <div className="wallet-book-modal">
        <div className="wallet-book-list">
          {rows.length === 0 ? (
            <div className="wallet-empty-row">No wallet months yet.</div>
          ) : rows.map(row => (
            <div key={row.id} className={`wallet-book-row ${row.month === currentMonth ? 'is-current' : ''}`}>
              <button type="button" className="wallet-book-main" onClick={() => onSummary(row)}>
                <strong>{formatMonthLabel(row.month)}</strong>
                <span>{row.month} / {row.status} / {row.allocation_count} allocations / {row.transaction_count} tx</span>
              </button>
              <div className="wallet-book-money">
                <strong>{formatMoney(row.wallet_balance_cents)}</strong>
                <span className={row.variance_cents === 0 ? 'is-good' : 'is-warn'}>{formatMoney(row.variance_cents)} variance</span>
              </div>
              <div className="wallet-book-actions">
                <button type="button" className="btn-ghost" onClick={() => onView(row.month)} title="View month">
                  <WalletIcon size={12} />
                </button>
                <button type="button" className="btn-ghost" onClick={() => onSummary(row)} title="View summary">
                  <ClipboardList size={12} />
                </button>
                <button type="button" className="btn-ghost" onClick={() => onEdit(row)} title={row.status === 'closed' ? 'Reopen to edit' : 'Edit balances'}>
                  <Pencil size={12} />
                </button>
                <button type="button" className="btn-ghost" onClick={() => onReports(row.month)} title="Open reports">
                  <BarChart3 size={12} />
                </button>
                <button type="button" className="btn-ghost wallet-danger-btn" onClick={() => onDelete(row)} title="Delete month">
                  <Trash2 size={12} />
                </button>
              </div>
            </div>
          ))}
        </div>

        {selectedRow && (
          <section className="wallet-book-summary">
            <div className="wallet-panel-header">
              <div>
                <span className="wallet-section-label">Month Summary</span>
                <strong>{formatMonthLabel(selectedRow.month)}</strong>
              </div>
              <span className="wallet-book-status">{selectedRow.status}</span>
            </div>
            <div className="wallet-book-summary-grid">
              <SummaryMetric label="Opening" value={selectedRow.opening_balance_cents} />
              <SummaryMetric label="Income" value={selectedRow.income_total_cents} tone="good" />
              <SummaryMetric label="Spending" value={selectedRow.spending_total_cents} />
              <SummaryMetric label="Wallet" value={selectedRow.wallet_balance_cents} />
              <SummaryMetric label="Expected" value={selectedRow.expected_balance_cents} />
              <SummaryMetric label="Variance" value={selectedRow.variance_cents} tone={selectedRow.variance_cents === 0 ? 'good' : 'warn'} />
              <SummaryMetric label="Reserved" value={selectedRow.total_reserved_cents} />
              <SummaryMetric label="Available" value={selectedRow.available_balance_cents} tone={selectedRow.available_balance_cents < 0 ? 'bad' : 'good'} />
            </div>
          </section>
        )}

        {editForm && (
          <section className="wallet-book-edit">
            <div className="wallet-panel-header">
              <div>
                <span className="wallet-section-label">Edit Month</span>
                <strong>{formatMonthLabel(editForm.month)}</strong>
              </div>
            </div>
            {editForm.status === 'closed' ? (
              <div className="wallet-book-closed-edit">
                <div className="wallet-empty-row">This month is closed. Reopen it before editing balances.</div>
                <button type="button" className="btn-ghost" onClick={() => onReopen(editForm.month)} disabled={saving}>
                  <RefreshCw size={13} />
                  Reopen to edit
                </button>
              </div>
            ) : (
              <form onSubmit={onSaveEdit} className="wallet-book-edit-form">
                <WalletField error={editErrors.opening}>
                  <input
                    value={editForm.opening}
                    onChange={event => setEditForm(prev => ({ ...prev, opening: event.target.value }))}
                    placeholder="Opening balance"
                    inputMode="decimal"
                    {...withFieldError(editErrors.opening)}
                  />
                </WalletField>
                <WalletField error={editErrors.wallet}>
                  <input
                    value={editForm.wallet}
                    onChange={event => setEditForm(prev => ({ ...prev, wallet: event.target.value }))}
                    placeholder="Wallet balance"
                    inputMode="decimal"
                    {...withFieldError(editErrors.wallet)}
                  />
                </WalletField>
                <button type="submit" className="btn-primary" disabled={saving || hasErrors(editErrors)}>
                  <Save size={14} />
                  Save
                </button>
              </form>
            )}
          </section>
        )}
      </div>
    </WalletModal>
  )
}

function MonthDeleteConfirmModal({ target, value, setValue, saving, onClose, onConfirm }) {
  if (!target) return null
  const confirmed = value === target.month
  return (
    <WalletModal title="Delete Wallet Month" onClose={onClose}>
      <form onSubmit={onConfirm} className="wallet-delete-confirm-form">
        <div className="wallet-alert wallet-danger-alert">
          <AlertTriangle size={14} />
          <span>Deleting {formatMonthLabel(target.month)} removes the month, income, allocations, transactions, splits, balance updates, and reconciliation records.</span>
        </div>
        <label>
          <span>Type {target.month} to confirm</span>
          <input value={value} onChange={event => setValue(event.target.value)} placeholder={target.month} />
        </label>
        <div className="wallet-modal-actions">
          <button type="button" className="btn-ghost" onClick={onClose} disabled={saving}>Cancel</button>
          <button type="submit" className="btn-primary wallet-danger-action" disabled={saving || !confirmed}>
            <Trash2 size={14} />
            Delete Month
          </button>
        </div>
      </form>
    </WalletModal>
  )
}

export default function WalletRoute() {
  const location = useLocation()
  const navigate = useNavigate()
  const section = location.pathname === '/wallet/settings'
    ? 'settings'
    : location.pathname === '/wallet/review'
      ? 'review'
      : location.pathname === '/wallet/reports'
        ? 'reports'
        : 'month'
  const isSettings = section === 'settings'
  const isReview = section === 'review'
  const isReports = section === 'reports'
  const [monthKey, setMonthKey] = useState(currentMonthKey())
  const [months, setMonths] = useState([])
  const [monthBookRows, setMonthBookRows] = useState([])
  const [monthBookOpen, setMonthBookOpen] = useState(false)
  const [bookSummaryMonth, setBookSummaryMonth] = useState('')
  const [bookEditForm, setBookEditForm] = useState(null)
  const [bookDeleteTarget, setBookDeleteTarget] = useState(null)
  const [bookDeleteConfirm, setBookDeleteConfirm] = useState('')
  const [summary, setSummary] = useState(null)
  const [settings, setSettings] = useState(null)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState(null)
  const [createForm, setCreateForm] = useState({ opening: '', wallet: '', useTemplates: true, carryForward: true })
  const [monthPreview, setMonthPreview] = useState(null)
  const [balanceInput, setBalanceInput] = useState('')
  const [allocationForm, setAllocationForm] = useState({ name: '', amount: '', type: 'flexible' })
  const [allocationTouched, setAllocationTouched] = useState({})
  const [allocationBudgetEdit, setAllocationBudgetEdit] = useState(null)
  const [incomeForm, setIncomeForm] = useState({ name: '', amount: '', receivedDate: localDateKey(), notes: '' })
  const [incomeTouched, setIncomeTouched] = useState({})
  const [incomeEdit, setIncomeEdit] = useState(null)
  const [templateForm, setTemplateForm] = useState({ name: '', amount: '', type: 'flexible', carryForward: false, defaultCategoryIds: [] })
  const [incomeTemplateForm, setIncomeTemplateForm] = useState({ name: '', amount: '', defaultDay: '' })
  const [categoryForm, setCategoryForm] = useState({ name: '' })
  const [reconcileOpen, setReconcileOpen] = useState(false)
  const [adjustmentReason, setAdjustmentReason] = useState('rounding')
  const [adjustmentNote, setAdjustmentNote] = useState('')
  const [reviewFilters, setReviewFilters] = useState({ cleanup: true, unsorted: false, rounded: false })
  const [reviewTransactions, setReviewTransactions] = useState([])
  const [detailAllocationId, setDetailAllocationId] = useState('')
  const [allocationDetail, setAllocationDetail] = useState(null)
  const [allocationDetailForm, setAllocationDetailForm] = useState(null)
  const [allocationDetailTransactionForm, setAllocationDetailTransactionForm] = useState({
    amount: '',
    categoryId: '',
    date: localDateKey(),
    note: '',
    rounded: false,
  })
  const [allocationDetailTransactionTouched, setAllocationDetailTransactionTouched] = useState({})
  const [reportFrom, setReportFrom] = useState(currentMonthKey())
  const [reportTo, setReportTo] = useState(currentMonthKey())
  const [reportAllocationFilter, setReportAllocationFilter] = useState('')
  const [monthlyReport, setMonthlyReport] = useState([])
  const [allocationReport, setAllocationReport] = useState([])
  const [categoryReport, setCategoryReport] = useState([])
  const [reviewReport, setReviewReport] = useState(null)
  const [splitTransaction, setSplitTransaction] = useState(null)
  const [splitDetail, setSplitDetail] = useState(null)
  const [splitRows, setSplitRows] = useState([])
  const [transactionForm, setTransactionForm] = useState({
    amount: '',
    allocationId: '',
    categoryId: '',
    date: localDateKey(),
    note: '',
    rounded: false,
  })
  const [transactionTouched, setTransactionTouched] = useState({})
  const [transactionMoreOpen, setTransactionMoreOpen] = useState(false)
  const [captureSheetOpen, setCaptureSheetOpen] = useState(false)
  const [lastTransactionNotice, setLastTransactionNotice] = useState(null)
  const [transactionAmountEdit, setTransactionAmountEdit] = useState(null)
  const transactionNoticeTimerRef = useRef(null)
  const transactionAmountInputRef = useRef(null)
  const mobileTransactionAmountInputRef = useRef(null)
  const sheetTransactionAmountInputRef = useRef(null)

  const load = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const [monthList, monthSummary, walletSettings, bookRows] = await Promise.all([
        api.get('/api/wallet/months'),
        api.get(`/api/wallet/months/${monthKey}/summary`).catch(err => {
          if (err.status === 404) return null
          throw err
        }),
        api.get('/api/wallet/settings'),
        api.get('/api/wallet/months/book'),
      ])
      setMonths(monthList)
      setMonthBookRows(bookRows)
      setSummary(monthSummary)
      setSettings(walletSettings)
      setBalanceInput(monthSummary ? moneyInputValue(monthSummary.wallet_balance_cents) : '')
      if (monthSummary) setMonthPreview(null)
    } catch (err) {
      setError(err.message)
    } finally {
      setLoading(false)
    }
  }, [monthKey])

  useEffect(() => {
    load()
  }, [load])

  const activeAllocations = useMemo(
    () => (summary?.allocations || []).filter(allocation => allocation.active),
    [summary]
  )

  const monthClosed = summary?.month?.status === 'closed'

  const unsortedCategory = useMemo(
    () => (summary?.categories || []).find(category => category.system_key === 'unsorted') || summary?.categories?.[0],
    [summary]
  )

  const selectedTransactionAllocation = useMemo(
    () => activeAllocations.find(allocation => allocation.id === transactionForm.allocationId) || activeAllocations[0],
    [activeAllocations, transactionForm.allocationId]
  )

  const quickCategoryOptions = useMemo(
    () => orderedCategoriesForAllocation(selectedTransactionAllocation, summary?.categories || []),
    [selectedTransactionAllocation, summary]
  )

  const recentTransactionChips = useMemo(() => {
    const allocationById = new Map(activeAllocations.map(allocation => [allocation.id, allocation]))
    const categoryById = new Map((summary?.categories || []).map(category => [category.id, category]))
    const seen = new Set()
    const chips = []
    for (const transaction of summary?.recent_transactions || []) {
      if (!transaction.allocation_id || !transaction.category_id) continue
      const key = `${transaction.allocation_id}:${transaction.category_id}`
      if (seen.has(key)) continue
      const allocation = allocationById.get(transaction.allocation_id)
      const category = categoryById.get(transaction.category_id)
      if (!allocation || !category) continue
      seen.add(key)
      chips.push({
        allocationId: allocation.id,
        categoryId: category.id,
        allocationName: allocation.name,
        categoryName: category.name,
        label: `${allocation.name} / ${category.name}`,
      })
      if (chips.length >= 5) break
    }
    return chips
  }, [activeAllocations, summary])

  const createErrors = useMemo(() => ({
    opening: validateAmount(createForm.opening, 'Opening balance', { nonNegative: true }),
    wallet: createForm.wallet.trim()
      ? validateAmount(createForm.wallet, 'Wallet balance', { nonNegative: true })
      : '',
  }), [createForm])

  const balanceErrors = useMemo(() => ({
    amount: validateAmount(balanceInput, 'Wallet balance', { required: true, nonNegative: true }),
  }), [balanceInput])

  const allocationErrors = useMemo(() => ({
    name: validateName(allocationForm.name, 'Allocation name'),
    amount: validateAmount(allocationForm.amount, 'Allocation amount', { nonNegative: true }),
  }), [allocationForm])

  const allocationBudgetEditErrors = useMemo(() => ({
    amount: validateAmount(allocationBudgetEdit?.amount || '', 'Initial allocation', { required: true, nonNegative: true }),
  }), [allocationBudgetEdit])

  const incomeErrors = useMemo(() => ({
    name: validateName(incomeForm.name, 'Income name'),
    amount: validateAmount(incomeForm.amount, 'Income amount', { nonNegative: true }),
    receivedDate: validateDate(incomeForm.receivedDate, 'Received date', false),
  }), [incomeForm])

  const incomeEditErrors = useMemo(() => ({
    name: validateName(incomeEdit?.name || '', 'Income name'),
    amount: validateAmount(incomeEdit?.amount || '', 'Income amount', { nonNegative: true }),
    receivedDate: validateDate(incomeEdit?.receivedDate || '', 'Received date', false),
  }), [incomeEdit])

  const transactionErrors = useMemo(() => ({
    amount: validateAmount(transactionForm.amount, 'Transaction amount', { required: true, positive: true }),
    allocation: transactionForm.allocationId || activeAllocations.length === 0 ? '' : 'Allocation is required',
    category: transactionForm.categoryId || (summary?.categories || []).length === 0 ? '' : 'Category is required',
    date: validateDate(transactionForm.date, 'Transaction date'),
  }), [transactionForm, activeAllocations.length, summary])

  const transactionAmountEditErrors = useMemo(() => ({
    amount: validateAmount(transactionAmountEdit?.amount || '', 'Transaction amount', { required: true, positive: true }),
  }), [transactionAmountEdit])

  const incomeDisplayErrors = useMemo(() => ({
    name: visibleFieldError(incomeErrors, incomeTouched, 'name'),
    amount: visibleFieldError(incomeErrors, incomeTouched, 'amount'),
    receivedDate: visibleFieldError(incomeErrors, incomeTouched, 'receivedDate'),
  }), [incomeErrors, incomeTouched])

  const incomeEditDisplayErrors = useMemo(() => ({
    name: visibleFieldError(incomeEditErrors, incomeEdit?.touched || {}, 'name'),
    amount: visibleFieldError(incomeEditErrors, incomeEdit?.touched || {}, 'amount'),
    receivedDate: visibleFieldError(incomeEditErrors, incomeEdit?.touched || {}, 'receivedDate'),
  }), [incomeEditErrors, incomeEdit])

  const allocationDisplayErrors = useMemo(() => ({
    name: visibleFieldError(allocationErrors, allocationTouched, 'name'),
    amount: visibleFieldError(allocationErrors, allocationTouched, 'amount'),
  }), [allocationErrors, allocationTouched])

  const allocationBudgetEditDisplayErrors = useMemo(() => ({
    amount: visibleFieldError(allocationBudgetEditErrors, allocationBudgetEdit?.touched || {}, 'amount'),
  }), [allocationBudgetEditErrors, allocationBudgetEdit])

  const transactionDisplayErrors = useMemo(() => ({
    amount: visibleFieldError(transactionErrors, transactionTouched, 'amount'),
    allocation: visibleFieldError(transactionErrors, transactionTouched, 'allocation'),
    category: visibleFieldError(transactionErrors, transactionTouched, 'category'),
    date: visibleFieldError(transactionErrors, transactionTouched, 'date'),
  }), [transactionErrors, transactionTouched])

  const transactionAmountEditDisplayErrors = useMemo(() => ({
    amount: visibleFieldError(transactionAmountEditErrors, transactionAmountEdit?.touched || {}, 'amount'),
  }), [transactionAmountEditErrors, transactionAmountEdit])

  const previewErrors = useMemo(() => {
    const income = (monthPreview?.income_items || []).map(item => ({
      name: validateName(item.name, 'Income name'),
      amount: validateAmount(item.amount_input ?? moneyInputValue(item.amount_cents), 'Income amount', { nonNegative: true }),
      receivedDate: validateDate(item.received_date || '', 'Received date', false),
      appliesToMonth: validateMonth(item.applies_to_month, 'Applies month'),
    }))
    const allocations = (monthPreview?.allocations || []).map(allocation => ({
      name: validateName(allocation.name, 'Allocation name'),
      amount: validateAmount(allocation.amount_input ?? moneyInputValue(allocation.budgeted_cents), 'Allocation budget', { nonNegative: true }),
    }))
    return { income, allocations }
  }, [monthPreview])

  const previewInvalid = useMemo(
    () => previewErrors.income.some(hasErrors) || previewErrors.allocations.some(hasErrors),
    [previewErrors]
  )

  const parsedBalanceInputCents = useMemo(() => {
    if (balanceInput.trim() === '') return null
    try {
      return parseCents(balanceInput, 'Wallet balance')
    } catch {
      return null
    }
  }, [balanceInput])

  const balanceDifference = useMemo(() => {
    if (!summary || parsedBalanceInputCents === null) return null
    return parsedBalanceInputCents - summary.expected_balance_cents
  }, [parsedBalanceInputCents, summary])

  const balanceChanged = useMemo(() => {
    if (!summary || parsedBalanceInputCents === null) return false
    return parsedBalanceInputCents !== summary.wallet_balance_cents
  }, [parsedBalanceInputCents, summary])

  const touchIncome = (field) => {
    setIncomeTouched(prev => ({ ...prev, [field]: true }))
  }

  const touchAllocation = (field) => {
    setAllocationTouched(prev => ({ ...prev, [field]: true }))
  }

  const touchTransaction = (field) => {
    setTransactionTouched(prev => ({ ...prev, [field]: true }))
  }

  const clearTransactionNoticeTimer = useCallback(() => {
    if (!transactionNoticeTimerRef.current) return
    window.clearTimeout(transactionNoticeTimerRef.current)
    transactionNoticeTimerRef.current = null
  }, [])

  const showTransactionNotice = useCallback((notice) => {
    clearTransactionNoticeTimer()
    setLastTransactionNotice(notice)
    transactionNoticeTimerRef.current = window.setTimeout(() => {
      setLastTransactionNotice(current => current?.createdAt === notice.createdAt ? null : current)
      transactionNoticeTimerRef.current = null
    }, 8000)
  }, [clearTransactionNoticeTimer])

  const focusTransactionAmount = useCallback((target = 'auto') => {
    window.requestAnimationFrame(() => {
      const input = target === 'sheet'
        ? sheetTransactionAmountInputRef.current
        : target === 'mobile'
          ? mobileTransactionAmountInputRef.current
          : target === 'desktop'
            ? transactionAmountInputRef.current
            : captureSheetOpen
              ? sheetTransactionAmountInputRef.current
              : mobileTransactionAmountInputRef.current || transactionAmountInputRef.current
      input?.focus()
      input?.select?.()
    })
  }, [captureSheetOpen])

  const selectRecentTransactionChip = useCallback((chip, focusTarget = 'auto') => {
    setTransactionForm(prev => ({
      ...prev,
      allocationId: chip.allocationId,
      categoryId: chip.categoryId,
    }))
    focusTransactionAmount(focusTarget)
  }, [focusTransactionAmount])

  useEffect(() => () => clearTransactionNoticeTimer(), [clearTransactionNoticeTimer])

  useEffect(() => {
    if (captureSheetOpen) focusTransactionAmount('sheet')
  }, [captureSheetOpen, focusTransactionAmount])

  useEffect(() => {
    if (!captureSheetOpen) return undefined
    const handleKeyDown = (event) => {
      if (event.key === 'Escape') setCaptureSheetOpen(false)
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [captureSheetOpen])

  useEffect(() => {
    if (isSettings || isReview || isReports || !summary) setCaptureSheetOpen(false)
  }, [isSettings, isReview, isReports, summary])

  useEffect(() => {
    if (!summary) return
    const firstCategoryID = quickCategoryOptions[0]?.id || unsortedCategory?.id || ''
    setTransactionForm(prev => ({
      ...prev,
      allocationId: prev.allocationId || activeAllocations[0]?.id || '',
      categoryId: quickCategoryOptions.some(category => category.id === prev.categoryId) ? prev.categoryId : firstCategoryID,
    }))
  }, [summary, activeAllocations, unsortedCategory, quickCategoryOptions])

  useEffect(() => {
    if (!summary || !isReview) return
    const workExpense = activeAllocations.find(allocation => allocation.name.toLowerCase() === 'work expense')
    setDetailAllocationId(prev => prev || workExpense?.id || activeAllocations[0]?.id || '')
  }, [summary, isReview, activeAllocations])

  const loadReview = useCallback(async () => {
    if (!isReview || !summary) return
    setError(null)
    try {
      const params = new URLSearchParams({
        cleanup: String(reviewFilters.cleanup),
        unsorted: String(reviewFilters.unsorted),
        rounded: String(reviewFilters.rounded),
      })
      const result = await api.get(`/api/wallet/months/${monthKey}/review?${params.toString()}`)
      setReviewTransactions((result.transactions || []).map(transaction => ({
        ...transaction,
        amount_input: moneyInputValue(transaction.amount_cents),
        note_input: transaction.note || '',
      })))
    } catch (err) {
      setError(err.message)
    }
  }, [isReview, monthKey, reviewFilters, summary])

  useEffect(() => {
    loadReview()
  }, [loadReview])

  const loadAllocationDetail = useCallback(async () => {
    if (!isReview || !detailAllocationId) return
    setError(null)
    try {
      const detail = await api.get(`/api/wallet/allocations/${detailAllocationId}/detail`)
      setAllocationDetail(detail)
    } catch (err) {
      setError(err.message)
    }
  }, [detailAllocationId, isReview])

  useEffect(() => {
    loadAllocationDetail()
  }, [loadAllocationDetail])

  useEffect(() => {
    if (!allocationDetail) {
      setAllocationDetailForm(null)
      return
    }
    setAllocationDetailForm({
      name: allocationDetail.allocation.name,
      amount: moneyInputValue(allocationDetail.allocation.budgeted_cents),
      type: allocationDetail.allocation.type,
      carryForward: !!allocationDetail.allocation.carry_forward,
      active: !!allocationDetail.allocation.active,
    })
    const detailCategories = orderedCategoriesForAllocation(allocationDetail.allocation, summary?.categories || [])
    setAllocationDetailTransactionForm(prev => ({
      ...prev,
      categoryId: detailCategories.some(category => category.id === prev.categoryId) ? prev.categoryId : detailCategories[0]?.id || '',
      date: prev.date || localDateKey(),
    }))
  }, [allocationDetail, summary])

  const loadReports = useCallback(async () => {
    if (!isReports) return
    setError(null)
    try {
      const monthlyParams = new URLSearchParams({ from: reportFrom || monthKey, to: reportTo || monthKey })
      const categoryParams = new URLSearchParams({ month: monthKey })
      if (reportAllocationFilter) categoryParams.set('allocation_id', reportAllocationFilter)
      const [monthly, allocations, categories, review] = await Promise.all([
        api.get(`/api/wallet/reports/monthly?${monthlyParams.toString()}`),
        api.get(`/api/wallet/reports/allocations?month=${monthKey}`).catch(err => err.status === 404 ? [] : Promise.reject(err)),
        api.get(`/api/wallet/reports/categories?${categoryParams.toString()}`).catch(err => err.status === 404 ? [] : Promise.reject(err)),
        api.get(`/api/wallet/reports/review?month=${monthKey}`).catch(err => err.status === 404 ? null : Promise.reject(err)),
      ])
      setMonthlyReport(monthly)
      setAllocationReport(allocations)
      setCategoryReport(categories)
      setReviewReport(review)
    } catch (err) {
      setError(err.message)
    }
  }, [isReports, monthKey, reportFrom, reportTo, reportAllocationFilter])

  useEffect(() => {
    loadReports()
  }, [loadReports])

  const openMonthBook = () => {
    setBookSummaryMonth(monthKey || monthBookRows[0]?.month || '')
    setMonthBookOpen(true)
  }

  const viewBookMonth = (selectedMonth) => {
    setMonthKey(selectedMonth)
    setMonthBookOpen(false)
    setBookEditForm(null)
    navigate('/wallet')
  }

  const reportBookMonth = (selectedMonth) => {
    setMonthKey(selectedMonth)
    setMonthBookOpen(false)
    setBookEditForm(null)
    navigate('/wallet/reports')
  }

  const summarizeBookMonth = (row) => {
    setBookSummaryMonth(row.month)
    setBookEditForm(null)
  }

  const editBookMonth = (row) => {
    setBookSummaryMonth(row.month)
    setBookEditForm({
      month: row.month,
      status: row.status,
      opening: moneyInputValue(row.opening_balance_cents),
      wallet: moneyInputValue(row.wallet_balance_cents),
    })
  }

  const saveBookMonthEdit = async (event) => {
    event.preventDefault()
    if (!bookEditForm || bookEditForm.status === 'closed') return
    const errors = {
      opening: validateAmount(bookEditForm.opening, 'Opening balance', { required: true, nonNegative: true }),
      wallet: validateAmount(bookEditForm.wallet, 'Wallet balance', { required: true, nonNegative: true }),
    }
    if (hasErrors(errors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/months/${bookEditForm.month}`, {
        opening_balance_cents: parseCents(bookEditForm.opening, 'Opening balance'),
        wallet_balance_cents: parseCents(bookEditForm.wallet, 'Wallet balance'),
      })
      setBookEditForm(null)
      await load()
      if (isReports) await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const reopenBookMonth = async (selectedMonth) => {
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${selectedMonth}/reopen`, {})
      setBookEditForm(prev => prev?.month === selectedMonth ? { ...prev, status: 'open' } : prev)
      await load()
      if (isReports) await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const requestDeleteBookMonth = (row) => {
    setBookDeleteTarget(row)
    setBookDeleteConfirm('')
  }

  const confirmDeleteBookMonth = async (event) => {
    event.preventDefault()
    if (!bookDeleteTarget || bookDeleteConfirm !== bookDeleteTarget.month) return
    const deletedMonth = bookDeleteTarget.month
    const remainingRows = monthBookRows.filter(row => row.month !== deletedMonth)
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/months/${deletedMonth}`)
      setBookDeleteTarget(null)
      setBookDeleteConfirm('')
      setBookEditForm(prev => prev?.month === deletedMonth ? null : prev)
      setBookSummaryMonth(prev => prev === deletedMonth ? remainingRows[0]?.month || '' : prev)
      setMonthBookRows(remainingRows)
      setMonths(prev => prev.filter(month => month.month !== deletedMonth))
      if (deletedMonth === monthKey) {
        setSummary(null)
        const nextMonth = remainingRows[0]?.month || currentMonthKey()
        setMonthKey(nextMonth)
        if (nextMonth === monthKey) await load()
      } else {
        await load()
        if (isReports) await loadReports()
      }
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitCreateMonth = async (event) => {
    event.preventDefault()
    if (hasErrors(createErrors)) return
    setSaving(true)
    setError(null)
    try {
      const opening = parseCents(createForm.opening, 'Opening balance')
      const wallet = createForm.wallet.trim() ? parseCents(createForm.wallet, 'Wallet balance') : opening
      const preview = await api.post('/api/wallet/months/preview', {
        month: monthKey,
        opening_balance_cents: opening,
        wallet_balance_cents: wallet,
        use_templates: createForm.useTemplates,
        carry_forward: createForm.carryForward,
      })
      setMonthPreview({
        ...preview,
        income_items: (preview.income_items || []).map(item => ({
          ...item,
          amount_input: moneyInputValue(item.amount_cents),
        })),
        allocations: (preview.allocations || []).map(allocation => ({
          ...allocation,
          amount_input: moneyInputValue(allocation.budgeted_cents),
          default_category_ids: categoryIds(allocation.default_categories || []),
        })),
      })
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const updatePreviewIncome = (index, patch) => {
    setMonthPreview(prev => ({
      ...prev,
      income_items: (prev?.income_items || []).map((item, i) => i === index ? { ...item, ...patch } : item),
    }))
  }

  const updatePreviewAllocation = (index, patch) => {
    setMonthPreview(prev => ({
      ...prev,
      allocations: (prev?.allocations || []).map((allocation, i) => i === index ? { ...allocation, ...patch } : allocation),
    }))
  }

  const confirmCreateMonth = async () => {
    if (!monthPreview || previewInvalid) return
    setSaving(true)
    setError(null)
    try {
      await api.post('/api/wallet/months', {
        month: monthPreview.month.month,
        opening_balance_cents: monthPreview.month.opening_balance_cents,
        wallet_balance_cents: monthPreview.month.wallet_balance_cents,
        income_items: (monthPreview.income_items || []).map(item => ({
          name: item.name,
          amount_cents: parseCents(item.amount_input ?? moneyInputValue(item.amount_cents), 'Income amount'),
          received_date: item.received_date || null,
          applies_to_month: item.applies_to_month || monthPreview.month.month,
          notes: item.notes || null,
        })),
        allocations: (monthPreview.allocations || []).map(allocation => ({
          template_id: allocation.template_id || null,
          name: allocation.name,
          budgeted_cents: parseCents(allocation.amount_input ?? moneyInputValue(allocation.budgeted_cents), 'Allocation amount'),
          type: allocation.type,
          carry_forward: !!allocation.carry_forward,
          active: allocation.active !== false,
          sort_order: Number(allocation.sort_order || 0),
          default_category_ids: allocation.default_category_ids || categoryIds(allocation.default_categories || []),
        })),
      })
      setCreateForm({ opening: '', wallet: '', useTemplates: true, carryForward: true })
      setMonthPreview(null)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitBalance = async (event) => {
    event.preventDefault()
    if (!summary) return
    if (hasErrors(balanceErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/months/${monthKey}`, {
        wallet_balance_cents: parseCents(balanceInput, 'Wallet balance'),
      })
      await load()
      if (isReports) await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitReconciliation = async (event) => {
    event.preventDefault()
    if (!summary) return
    if (hasErrors(balanceErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/balance-updates`, {
        new_balance_cents: parseCents(balanceInput, 'Wallet balance'),
        note: adjustmentNote || null,
        create_adjustment: true,
        adjustment_reason: adjustmentReason,
        adjustment_note: adjustmentNote || null,
      })
      setAdjustmentNote('')
      setReconcileOpen(false)
      await load()
      if (isReports) await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitAllocation = async (event) => {
    event.preventDefault()
    setAllocationTouched(prev => ({ ...prev, submit: true }))
    if (hasErrors(allocationErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/allocations`, {
        name: allocationForm.name,
        budgeted_cents: parseCents(allocationForm.amount, 'Allocation amount'),
        type: allocationForm.type,
        sort_order: (summary?.allocations?.length || 0) + 1,
      })
      setAllocationForm({ name: '', amount: '', type: 'flexible' })
      setAllocationTouched({})
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const startAllocationBudgetEdit = (allocation) => {
    setAllocationBudgetEdit({
      id: allocation.id,
      amount: moneyInputValue(allocation.budgeted_cents),
      touched: {},
    })
  }

  const updateAllocationBudgetEdit = (patch) => {
    setAllocationBudgetEdit(prev => prev ? { ...prev, ...patch } : prev)
  }

  const saveAllocationBudgetEdit = async (event) => {
    event.preventDefault()
    if (!allocationBudgetEdit) return
    setAllocationBudgetEdit(prev => prev ? { ...prev, touched: { ...(prev.touched || {}), submit: true } } : prev)
    if (hasErrors(allocationBudgetEditErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/allocations/${allocationBudgetEdit.id}`, {
        budgeted_cents: parseCents(allocationBudgetEdit.amount, 'Initial allocation'),
      })
      setAllocationBudgetEdit(null)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitIncome = async (event) => {
    event.preventDefault()
    setIncomeTouched(prev => ({ ...prev, submit: true }))
    if (hasErrors(incomeErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/income`, {
        name: incomeForm.name,
        amount_cents: parseCents(incomeForm.amount, 'Income amount'),
        received_date: incomeForm.receivedDate || null,
        applies_to_month: monthKey,
        notes: incomeForm.notes || null,
      })
      setIncomeForm({ name: '', amount: '', receivedDate: localDateKey(), notes: '' })
      setIncomeTouched({})
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const startIncomeEdit = (item) => {
    setIncomeEdit({
      id: item.id,
      name: item.name,
      amount: moneyInputValue(item.amount_cents),
      receivedDate: item.received_date || '',
      notes: item.notes || '',
      touched: {},
    })
  }

  const updateIncomeEdit = (patch) => {
    setIncomeEdit(prev => prev ? { ...prev, ...patch } : prev)
  }

  const touchIncomeEdit = (field) => {
    setIncomeEdit(prev => prev ? { ...prev, touched: { ...(prev.touched || {}), [field]: true } } : prev)
  }

  const saveIncomeEdit = async (event) => {
    event.preventDefault()
    if (!incomeEdit) return
    setIncomeEdit(prev => prev ? { ...prev, touched: { ...(prev.touched || {}), submit: true } } : prev)
    if (hasErrors(incomeEditErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/income/${incomeEdit.id}`, {
        name: incomeEdit.name,
        amount_cents: parseCents(incomeEdit.amount, 'Income amount'),
        received_date: incomeEdit.receivedDate || null,
        notes: incomeEdit.notes || null,
      })
      setIncomeEdit(null)
      await load()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitTransaction = async (event, focusTarget = 'auto') => {
    event.preventDefault()
    setTransactionTouched(prev => ({ ...prev, submit: true }))
    if (hasErrors(transactionErrors)) return
    setSaving(true)
    setError(null)
    try {
      const allocationId = transactionForm.allocationId || activeAllocations[0]?.id || ''
      const categoryId = transactionForm.categoryId || unsortedCategory?.id || ''
      const selectedAllocation = activeAllocations.find(allocation => allocation.id === allocationId)
      const selectedCategory = (summary?.categories || []).find(category => category.id === categoryId)
      const amountCents = parseCents(transactionForm.amount, 'Transaction amount')
      const created = await api.post(`/api/wallet/months/${monthKey}/transactions`, {
        allocation_id: allocationId,
        category_id: categoryId,
        date: transactionForm.date,
        amount_cents: amountCents,
        note: transactionForm.note || null,
        rounded: transactionForm.rounded,
      })
      const responseLabel = created?.allocation_name && created?.category_name
        ? `${created.allocation_name} / ${created.category_name}`
        : ''
      const fallbackLabel = selectedAllocation && selectedCategory
        ? `${selectedAllocation.name} / ${selectedCategory.name}`
        : ''
      showTransactionNotice({
        id: created?.id || null,
        label: responseLabel || fallbackLabel,
        amountCents: created?.amount_cents ?? amountCents,
        createdAt: Date.now(),
      })
      setTransactionForm(prev => ({
        ...prev,
        amount: '',
        date: localDateKey(),
        note: '',
        rounded: false,
      }))
      setTransactionTouched({})
      await load()
      focusTransactionAmount(focusTarget)
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const undoLastTransaction = async () => {
    const notice = lastTransactionNotice
    if (!notice?.id) return
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/transactions/${notice.id}`)
      clearTransactionNoticeTimer()
      setLastTransactionNotice(null)
      await load()
      if (isReview) {
        await loadReview()
        await loadAllocationDetail()
      }
      if (isReports) await loadReports()
    } catch (err) {
      setError(err.message)
      clearTransactionNoticeTimer()
      setLastTransactionNotice(null)
    } finally {
      setSaving(false)
    }
  }

  const startTransactionAmountEdit = (transaction) => {
    setTransactionAmountEdit({
      id: transaction.id,
      amount: moneyInputValue(transaction.amount_cents),
      touched: {},
    })
  }

  const updateTransactionAmountEdit = (patch) => {
    setTransactionAmountEdit(prev => prev ? { ...prev, ...patch } : prev)
  }

  const saveTransactionAmountEdit = async (event) => {
    event.preventDefault()
    if (!transactionAmountEdit) return
    setTransactionAmountEdit(prev => prev ? { ...prev, touched: { ...(prev.touched || {}), submit: true } } : prev)
    if (hasErrors(transactionAmountEditErrors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/transactions/${transactionAmountEdit.id}`, {
        amount_cents: parseCents(transactionAmountEdit.amount, 'Transaction amount'),
      })
      setTransactionAmountEdit(null)
      await load()
      await loadReview()
      await loadAllocationDetail()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const updateSettingsList = (key, id, patch) => {
    setSettings(prev => ({
      ...prev,
      [key]: (prev?.[key] || []).map(item => item.id === id ? { ...item, ...patch } : item),
    }))
  }

  const reorderSettingsList = async (key, orderedIds) => {
    const endpointBase = {
      allocation_templates: '/api/wallet/allocation-templates',
      income_templates: '/api/wallet/income-templates',
      categories: '/api/wallet/categories',
    }[key]
    const items = settings?.[key] || []
    if (!endpointBase || orderedIds.length !== items.length) return

    const byId = new Map(items.map(item => [item.id, item]))
    const reordered = orderedIds
      .map(id => byId.get(id))
      .filter(Boolean)
      .map((item, index) => ({
        ...item,
        sort_order: (index + 1) * 10,
        _wallet_original: null,
      }))
    if (reordered.length !== items.length) return

    setSettings(prev => ({
      ...prev,
      [key]: reordered,
    }))
    setSaving(true)
    setError(null)
    try {
      await Promise.all(reordered.map((item, index) => (
        api.patch(`${endpointBase}/${item.id}`, { sort_order: (index + 1) * 10 })
      )))
      await load()
    } catch (err) {
      setError(err.message)
      await load()
    } finally {
      setSaving(false)
    }
  }

  const submitAllocationTemplate = async (event) => {
    event.preventDefault()
    const errors = {
      name: validateName(templateForm.name, 'Allocation template name'),
      amount: validateAmount(templateForm.amount, 'Default amount', { nonNegative: true }),
    }
    if (hasErrors(errors)) return false
    setSaving(true)
    setError(null)
    try {
      await api.post('/api/wallet/allocation-templates', {
        name: templateForm.name,
        default_amount_cents: parseCents(templateForm.amount, 'Default amount'),
        type: templateForm.type,
        carry_forward: templateForm.carryForward,
        default_category_ids: templateForm.defaultCategoryIds || [],
        sort_order: (settings?.allocation_templates?.length || 0) + 1,
      })
      setTemplateForm({ name: '', amount: '', type: 'flexible', carryForward: false, defaultCategoryIds: [] })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const saveAllocationTemplate = async (template) => {
    const errors = {
      name: validateName(template.name, 'Allocation template name'),
      amount: validateAmount(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount', { nonNegative: true }),
    }
    if (hasErrors(errors)) return false
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/allocation-templates/${template.id}`, {
        name: template.name,
        default_amount_cents: parseCents(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount'),
        type: template.type,
        carry_forward: !!template.carry_forward,
        active: !!template.active,
        default_category_ids: templateCategoryIds(template),
        sort_order: template.sort_order || 0,
      })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const deleteAllocationTemplate = async (template) => {
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/allocation-templates/${template.id}`)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitIncomeTemplate = async (event) => {
    event.preventDefault()
    const errors = {
      name: validateName(incomeTemplateForm.name, 'Income template name'),
      amount: validateAmount(incomeTemplateForm.amount, 'Default amount', { nonNegative: true }),
      defaultDay: validateInteger(incomeTemplateForm.defaultDay, 'Default day', { min: 1, max: 31 }),
    }
    if (hasErrors(errors)) return false
    setSaving(true)
    setError(null)
    try {
      await api.post('/api/wallet/income-templates', {
        name: incomeTemplateForm.name,
        default_amount_cents: parseCents(incomeTemplateForm.amount, 'Default amount'),
        default_day: incomeTemplateForm.defaultDay ? Number(incomeTemplateForm.defaultDay) : null,
        sort_order: (settings?.income_templates?.length || 0) + 1,
      })
      setIncomeTemplateForm({ name: '', amount: '', defaultDay: '' })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const saveIncomeTemplate = async (template) => {
    const errors = {
      name: validateName(template.name, 'Income template name'),
      amount: validateAmount(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount', { nonNegative: true }),
      defaultDay: validateInteger(template.default_day_input ?? template.default_day ?? '', 'Default day', { min: 1, max: 31 }),
    }
    if (hasErrors(errors)) return false
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/income-templates/${template.id}`, {
        name: template.name,
        default_amount_cents: parseCents(template.amount_input ?? moneyInputValue(template.default_amount_cents), 'Default amount'),
        default_day: template.default_day_input !== undefined
          ? (template.default_day_input ? Number(template.default_day_input) : null)
          : template.default_day,
        active: !!template.active,
        sort_order: template.sort_order || 0,
      })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const deleteIncomeTemplate = async (template) => {
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/income-templates/${template.id}`)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitCategory = async (event) => {
    event.preventDefault()
    if (validateName(categoryForm.name, 'Category name')) return false
    setSaving(true)
    setError(null)
    try {
      await api.post('/api/wallet/categories', {
        name: categoryForm.name,
        sort_order: (settings?.categories?.length || 0) + 1,
      })
      setCategoryForm({ name: '' })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const saveCategory = async (category) => {
    if (validateName(category.name, 'Category name')) return false
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/categories/${category.id}`, {
        name: category.name,
        active: !!category.active,
        sort_order: category.sort_order || 0,
      })
      await load()
      return true
    } catch (err) {
      setError(err.message)
      return false
    } finally {
      setSaving(false)
    }
  }

  const deleteCategory = async (category) => {
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/categories/${category.id}`)
      await load()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const updateReviewTransaction = (id, patch) => {
    setReviewTransactions(prev => prev.map(transaction => (
      transaction.id === id ? { ...transaction, ...patch } : transaction
    )))
  }

  const saveReviewTransaction = async (transaction) => {
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/transactions/${transaction.id}`, {
        allocation_id: transaction.allocation_id,
        category_id: transaction.category_id,
        date: transaction.date,
        amount_cents: parseCents(transaction.amount_input ?? moneyInputValue(transaction.amount_cents), 'Transaction amount'),
        note: transaction.note_input || null,
        rounded: !!transaction.rounded,
      })
      await load()
      await loadReview()
      await loadAllocationDetail()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const saveAllocationDetail = async (event) => {
    event.preventDefault()
    if (!allocationDetail || !allocationDetailForm) return
    const errors = {
      name: validateName(allocationDetailForm.name, 'Allocation name'),
      amount: validateAmount(allocationDetailForm.amount, 'Allocation budget', { nonNegative: true }),
    }
    if (hasErrors(errors)) return
    setSaving(true)
    setError(null)
    try {
      await api.patch(`/api/wallet/allocations/${allocationDetail.allocation.id}`, {
        name: allocationDetailForm.name,
        budgeted_cents: parseCents(allocationDetailForm.amount, 'Allocation budget'),
        type: allocationDetailForm.type,
        carry_forward: !!allocationDetailForm.carryForward,
        active: !!allocationDetailForm.active,
      })
      await load()
      await loadReview()
      await loadAllocationDetail()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitAllocationDetailTransaction = async (event) => {
    event.preventDefault()
    if (!allocationDetail) return
    setAllocationDetailTransactionTouched(prev => ({ ...prev, submit: true }))
    const errors = {
      amount: validateAmount(allocationDetailTransactionForm.amount, 'Transaction amount', { required: true, positive: true }),
      date: validateDate(allocationDetailTransactionForm.date, 'Transaction date'),
      category: allocationDetailTransactionForm.categoryId ? '' : 'Category is required',
    }
    if (hasErrors(errors)) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/transactions`, {
        allocation_id: allocationDetail.allocation.id,
        category_id: allocationDetailTransactionForm.categoryId,
        date: allocationDetailTransactionForm.date,
        amount_cents: parseCents(allocationDetailTransactionForm.amount, 'Transaction amount'),
        note: allocationDetailTransactionForm.note || null,
        rounded: !!allocationDetailTransactionForm.rounded,
      })
      setAllocationDetailTransactionForm(prev => ({
        ...prev,
        amount: '',
        date: localDateKey(),
        note: '',
        rounded: false,
      }))
      setAllocationDetailTransactionTouched({})
      await load()
      await loadReview()
      await loadAllocationDetail()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const deleteTransaction = async (transaction) => {
    if (isSplitChild(transaction)) return
    const message = isSplitParent(transaction)
      ? 'Delete this split parent and the whole split group?'
      : 'Delete this transaction?'
    if (!window.confirm(message)) return
    setSaving(true)
    setError(null)
    try {
      await api.delete(`/api/wallet/transactions/${transaction.id}`)
      await load()
      await loadReview()
      await loadAllocationDetail()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const closeMonth = async () => {
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/close`, {})
      await load()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const reopenMonth = async () => {
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/months/${monthKey}/reopen`, {})
      await load()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const openSplitModal = (transaction) => {
    setSplitTransaction(transaction)
    const half = Math.floor(transaction.amount_cents / 2)
    const remainder = transaction.amount_cents - half
    setSplitRows([
      {
        amount: moneyInputValue(half),
        allocationId: transaction.allocation_id,
        categoryId: transaction.category_id || unsortedCategory?.id || '',
        note: transaction.note || '',
      },
      {
        amount: moneyInputValue(remainder),
        allocationId: transaction.allocation_id,
        categoryId: transaction.category_id || unsortedCategory?.id || '',
        note: '',
      },
    ])
  }

  const openSplitDetail = async (transaction) => {
    setSaving(true)
    setError(null)
    try {
      const detail = await api.get(`/api/wallet/transactions/${transaction.id}/split`)
      setSplitDetail(detail)
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const submitSplit = async (event) => {
    event.preventDefault()
    if (!splitTransaction) return
    setSaving(true)
    setError(null)
    try {
      await api.post(`/api/wallet/transactions/${splitTransaction.id}/split`, {
        splits: splitRows.map(row => ({
          allocation_id: row.allocationId,
          category_id: row.categoryId,
          date: splitTransaction.date,
          amount_cents: parseCents(row.amount, 'Split amount'),
          note: row.note || null,
          rounded: !!splitTransaction.rounded,
        })),
      })
      setSplitTransaction(null)
      setSplitRows([])
      await load()
      await loadReview()
      await loadAllocationDetail()
      await loadReports()
    } catch (err) {
      setError(err.message)
    } finally {
      setSaving(false)
    }
  }

  const walletOverlayOpen = reconcileOpen || !!splitTransaction || !!splitDetail || monthBookOpen || !!bookDeleteTarget || captureSheetOpen
  const showCaptureFab = section === 'month' && !!summary && !loading && !walletOverlayOpen

  return (
    <div className="wallet-page">
      <header className="wallet-header">
        <div className="wallet-header-left">
          <button
            type="button"
            className="btn-ghost wallet-mobile-menu-btn"
            onClick={() => window.dispatchEvent(new Event('toggle-sidebar'))}
            title="Open navigation"
            aria-label="Open navigation"
          >
            <Menu size={16} />
          </button>
          <div className="wallet-header-icon"><WalletIcon size={18} /></div>
          <div>
            <span className="wallet-eyebrow">Wallet</span>
            <h1>{formatMonthLabel(monthKey)}</h1>
          </div>
        </div>
        <div className="wallet-header-actions">
          <Link to="/wallet" className={`wallet-nav-btn ${section === 'month' ? 'active' : ''}`}>
            <WalletIcon size={14} />
            Month
          </Link>
          <Link to="/wallet/review" className={`wallet-nav-btn ${isReview ? 'active' : ''}`}>
            <ClipboardList size={14} />
            Review
          </Link>
          <Link to="/wallet/reports" className={`wallet-nav-btn ${isReports ? 'active' : ''}`}>
            <BarChart3 size={14} />
            Reports
          </Link>
          <Link to="/wallet/settings" className={`wallet-nav-btn ${isSettings ? 'active' : ''}`}>
            <Settings size={14} />
            Settings
          </Link>
          <button type="button" className="wallet-nav-btn" onClick={openMonthBook} disabled={loading} title="Open month book">
            <BookOpen size={14} />
            Months
          </button>
          <input
            type="month"
            value={monthKey}
            onChange={event => setMonthKey(event.target.value || currentMonthKey())}
            aria-label="Wallet month"
          />
          <button type="button" className="btn-ghost" onClick={load} disabled={loading} title="Refresh wallet">
            <RefreshCw size={14} />
            Refresh
          </button>
        </div>
      </header>

      {error && (
        <div className="wallet-alert">
          <AlertTriangle size={14} />
          <span>{error}</span>
        </div>
      )}

      <WalletNotice
        notice={lastTransactionNotice}
        saving={saving}
        onUndo={undoLastTransaction}
      />

      {loading ? (
        <div className="wallet-loading">Loading wallet...</div>
      ) : isSettings ? (
        <WalletSettingsView
          settings={settings}
          saving={saving}
          templateForm={templateForm}
          setTemplateForm={setTemplateForm}
          incomeTemplateForm={incomeTemplateForm}
          setIncomeTemplateForm={setIncomeTemplateForm}
          categoryForm={categoryForm}
          setCategoryForm={setCategoryForm}
          updateSettingsList={updateSettingsList}
          submitAllocationTemplate={submitAllocationTemplate}
          saveAllocationTemplate={saveAllocationTemplate}
          deleteAllocationTemplate={deleteAllocationTemplate}
          submitIncomeTemplate={submitIncomeTemplate}
          saveIncomeTemplate={saveIncomeTemplate}
          deleteIncomeTemplate={deleteIncomeTemplate}
          submitCategory={submitCategory}
          saveCategory={saveCategory}
          deleteCategory={deleteCategory}
          reorderSettingsList={reorderSettingsList}
        />
      ) : isReports ? (
        <WalletReportsView
          monthKey={monthKey}
          setMonthKey={setMonthKey}
          months={months}
          summary={summary}
          saving={saving}
          reportFrom={reportFrom}
          setReportFrom={setReportFrom}
          reportTo={reportTo}
          setReportTo={setReportTo}
          reportAllocationFilter={reportAllocationFilter}
          setReportAllocationFilter={setReportAllocationFilter}
          monthlyReport={monthlyReport}
          allocationReport={allocationReport}
          categoryReport={categoryReport}
          reviewReport={reviewReport}
          reloadReports={loadReports}
          closeMonth={closeMonth}
          reopenMonth={reopenMonth}
        />
      ) : !summary ? (
        <section className="wallet-empty">
          <div className="wallet-empty-icon"><CalendarDays size={20} /></div>
          <h2>Create {formatMonthLabel(monthKey)}</h2>
          <form onSubmit={submitCreateMonth} className="wallet-create-form">
            <label>
              <span>Opening Balance</span>
              <WalletField error={createErrors.opening}>
                <input
                  value={createForm.opening}
                  onChange={event => setCreateForm(prev => ({ ...prev, opening: event.target.value }))}
                  placeholder="0.00"
                  inputMode="decimal"
                  {...withFieldError(createErrors.opening)}
                />
              </WalletField>
            </label>
            <label>
              <span>Wallet Balance</span>
              <WalletField error={createErrors.wallet}>
                <input
                  value={createForm.wallet}
                  onChange={event => setCreateForm(prev => ({ ...prev, wallet: event.target.value }))}
                  placeholder="Same as opening"
                  inputMode="decimal"
                  {...withFieldError(createErrors.wallet)}
                />
              </WalletField>
            </label>
            <label className="wallet-create-check">
              <input
                type="checkbox"
                checked={createForm.useTemplates}
                onChange={event => setCreateForm(prev => ({ ...prev, useTemplates: event.target.checked }))}
              />
              <span>Use Templates</span>
            </label>
            <label className="wallet-create-check">
              <input
                type="checkbox"
                checked={createForm.carryForward}
                disabled={!createForm.useTemplates}
                onChange={event => setCreateForm(prev => ({ ...prev, carryForward: event.target.checked }))}
              />
              <span>Carry Forward</span>
            </label>
            <button type="submit" className="btn-primary" disabled={saving || hasErrors(createErrors)}>
              <Plus size={14} />
              Preview Month
            </button>
          </form>
          {monthPreview && (
            <MonthPreviewPanel
              preview={monthPreview}
              saving={saving}
              updatePreviewIncome={updatePreviewIncome}
              updatePreviewAllocation={updatePreviewAllocation}
              confirmCreateMonth={confirmCreateMonth}
              cancelPreview={() => setMonthPreview(null)}
              previewInvalid={previewInvalid}
              previewErrors={previewErrors}
            />
          )}
          {months.length > 0 && (
            <div className="wallet-month-hints">
              {months.slice(0, 6).map(month => (
                <button key={month.id} type="button" className="btn-ghost" onClick={() => setMonthKey(month.month)}>
                  {month.month}
                </button>
              ))}
            </div>
          )}
        </section>
      ) : isReview ? (
        <WalletReviewView
          summary={summary}
          saving={saving}
          monthClosed={monthClosed}
          reviewFilters={reviewFilters}
          setReviewFilters={setReviewFilters}
          reviewTransactions={reviewTransactions}
          updateReviewTransaction={updateReviewTransaction}
          saveReviewTransaction={saveReviewTransaction}
          reloadReview={loadReview}
          detailAllocationId={detailAllocationId}
          setDetailAllocationId={setDetailAllocationId}
          allocationDetail={allocationDetail}
          allocationDetailForm={allocationDetailForm}
          setAllocationDetailForm={setAllocationDetailForm}
          saveAllocationDetail={saveAllocationDetail}
          allocationDetailTransactionForm={allocationDetailTransactionForm}
          setAllocationDetailTransactionForm={setAllocationDetailTransactionForm}
          allocationDetailTransactionTouched={allocationDetailTransactionTouched}
          touchAllocationDetailTransaction={field => setAllocationDetailTransactionTouched(prev => ({ ...prev, [field]: true }))}
          submitAllocationDetailTransaction={submitAllocationDetailTransaction}
          openSplitModal={openSplitModal}
          openSplitDetail={openSplitDetail}
          deleteTransaction={deleteTransaction}
        />
      ) : (
        <>
          <section className="wallet-panel wallet-mobile-capture-panel">
            <div className="wallet-panel-header">
              <div>
                <span className="wallet-section-label">Quick Capture</span>
                <strong>Transaction</strong>
              </div>
              <ReceiptText size={16} />
            </div>
            <TransactionCaptureForm
              summary={summary}
              saving={saving}
              monthClosed={monthClosed}
              activeAllocations={activeAllocations}
              quickCategoryOptions={quickCategoryOptions}
              transactionForm={transactionForm}
              setTransactionForm={setTransactionForm}
              transactionDisplayErrors={transactionDisplayErrors}
              transactionErrors={transactionErrors}
              transactionMoreOpen={transactionMoreOpen}
              setTransactionMoreOpen={setTransactionMoreOpen}
              touchTransaction={touchTransaction}
              submitTransaction={event => submitTransaction(event, 'mobile')}
              recentChips={recentTransactionChips}
              onRecentChipSelect={chip => selectRecentTransactionChip(chip, 'mobile')}
              amountInputRef={mobileTransactionAmountInputRef}
              variant="mobile"
            />
          </section>

          <RecentTransactionsPanel
            transactions={summary.recent_transactions || []}
            saving={saving}
            monthClosed={monthClosed}
            transactionAmountEdit={transactionAmountEdit}
            transactionAmountEditDisplayErrors={transactionAmountEditDisplayErrors}
            transactionAmountEditErrors={transactionAmountEditErrors}
            startTransactionAmountEdit={startTransactionAmountEdit}
            updateTransactionAmountEdit={updateTransactionAmountEdit}
            setTransactionAmountEdit={setTransactionAmountEdit}
            saveTransactionAmountEdit={saveTransactionAmountEdit}
            openSplitModal={openSplitModal}
            openSplitDetail={openSplitDetail}
            deleteTransaction={deleteTransaction}
            className="wallet-mobile-recent-panel"
          />

          <section className="wallet-summary-strip">
            <SummaryMetric label="Opening" value={summary.month.opening_balance_cents} icon={Banknote} />
            <SummaryMetric label="Income" value={summary.income_total_cents} tone="good" icon={Banknote} />
            <SummaryMetric label="Wallet Balance" value={summary.wallet_balance_cents} icon={WalletIcon} />
            <SummaryMetric label="Total Reserved" value={summary.total_reserved_cents} icon={PiggyBank} />
            <SummaryMetric
              label="Available"
              value={summary.available_balance_cents}
              tone={summary.available_balance_cents < 0 ? 'bad' : 'good'}
              icon={Banknote}
            />
            <SummaryMetric
              label="Variance"
              value={summary.variance_cents}
              tone={summary.variance_cents === 0 ? 'good' : 'warn'}
              icon={AlertTriangle}
            />
          </section>

          <section className="wallet-balance-panel">
            <form onSubmit={submitBalance} className="wallet-balance-form">
              <label>
                <span>Wallet Balance</span>
                <WalletField error={balanceErrors.amount}>
                  <input
                    value={balanceInput}
                    onChange={event => setBalanceInput(event.target.value)}
                    inputMode="decimal"
                    disabled={monthClosed}
                    {...withFieldError(balanceErrors.amount)}
                  />
                </WalletField>
              </label>
              <button type="submit" className="btn-ghost" disabled={saving || monthClosed || !balanceChanged || hasErrors(balanceErrors)}>
                <Save size={13} />
                Save Balance
              </button>
              <button
                type="button"
                className="btn-ghost"
                disabled={saving || monthClosed || !balanceDifference || hasErrors(balanceErrors)}
                onClick={() => setReconcileOpen(true)}
              >
                <AlertTriangle size={13} />
                Reconcile
              </button>
            </form>
            <div className="wallet-review-strip">
              <span>{summary.month.status}</span>
              <span>Expected {formatMoney(summary.expected_balance_cents)}</span>
              {balanceDifference !== null && <span>Difference {formatMoney(balanceDifference)}</span>}
              <span>{summary.review_counts.unsorted_count} unsorted / {formatMoney(summary.review_counts.unsorted_cents)}</span>
              <span>{summary.review_counts.rounded_count} rounded / {formatMoney(summary.review_counts.rounded_cents)}</span>
            </div>
          </section>

          <div className="wallet-grid">
            <section className="wallet-panel wallet-allocations-panel">
              <div className="wallet-panel-header">
                <div>
                  <span className="wallet-section-label">Allocations</span>
                  <strong>{summary.allocations.length} envelopes</strong>
                </div>
              </div>
              <div className="wallet-allocation-list">
                {summary.allocations.length === 0 ? (
                  <div className="wallet-empty-row">No allocations yet.</div>
                ) : summary.allocations.map(allocation => {
                  const usage = allocation.budgeted_cents > 0
                    ? Math.min(100, Math.round((allocation.spent_cents / allocation.budgeted_cents) * 100))
                    : 0
                  const editingBudget = allocationBudgetEdit?.id === allocation.id
                  return (
                    <div key={allocation.id} className={`wallet-allocation-row ${allocation.remaining_cents < 0 ? 'is-overspent' : ''}`}>
                      <div className="wallet-allocation-main">
                        <strong>{allocation.name}</strong>
                        <span>{typeLabel(allocation.type)} / initial allocation {formatMoney(allocation.budgeted_cents)}</span>
                      </div>
                      <div className="wallet-allocation-side">
                        <div className="wallet-allocation-money">
                          <span className="wallet-money-label">{allocation.remaining_cents < 0 ? 'Overspent' : 'Amount left'}</span>
                          <strong>{formatMoney(allocation.remaining_cents)}</strong>
                          <span>{formatMoney(allocation.spent_cents)} spent</span>
                        </div>
                        <button
                          type="button"
                          className="btn-ghost wallet-allocation-edit-btn"
                          onClick={() => startAllocationBudgetEdit(allocation)}
                          disabled={saving || monthClosed}
                          title="Edit initial allocation"
                        >
                          <Pencil size={13} />
                        </button>
                      </div>
                      <div className="wallet-progress">
                        <i style={{ width: `${usage}%` }} />
                      </div>
                      {editingBudget && (
                        <form onSubmit={saveAllocationBudgetEdit} className="wallet-allocation-budget-form">
                          <WalletField error={allocationBudgetEditDisplayErrors.amount}>
                            <input
                              value={allocationBudgetEdit.amount}
                              onChange={event => updateAllocationBudgetEdit({ amount: event.target.value })}
                              onBlur={() => updateAllocationBudgetEdit({ touched: { ...(allocationBudgetEdit.touched || {}), amount: true } })}
                              inputMode="decimal"
                              aria-label={`${allocation.name} initial allocation amount`}
                              {...withFieldError(allocationBudgetEditDisplayErrors.amount)}
                            />
                          </WalletField>
                          <button type="button" className="btn-ghost" onClick={() => setAllocationBudgetEdit(null)} disabled={saving}>
                            Cancel
                          </button>
                          <button type="submit" className="btn-primary" disabled={saving || hasErrors(allocationBudgetEditErrors)}>
                            <Save size={13} />
                            Save
                          </button>
                        </form>
                      )}
                    </div>
                  )
                })}
              </div>
              <form onSubmit={submitAllocation} className="wallet-inline-form">
                <WalletField error={allocationDisplayErrors.name}>
                  <input
                    value={allocationForm.name}
                    onChange={event => setAllocationForm(prev => ({ ...prev, name: event.target.value }))}
                    onBlur={() => touchAllocation('name')}
                    placeholder="Allocation name"
                    {...withFieldError(allocationDisplayErrors.name)}
                  />
                </WalletField>
                <WalletField error={allocationDisplayErrors.amount}>
                  <input
                    value={allocationForm.amount}
                    onChange={event => setAllocationForm(prev => ({ ...prev, amount: event.target.value }))}
                    onBlur={() => touchAllocation('amount')}
                    placeholder="Amount"
                    inputMode="decimal"
                    {...withFieldError(allocationDisplayErrors.amount)}
                  />
                </WalletField>
                <select
                  value={allocationForm.type}
                  onChange={event => setAllocationForm(prev => ({ ...prev, type: event.target.value }))}
                >
                  <option value="flexible">Flexible</option>
                  <option value="fixed">Fixed</option>
                  <option value="sinking_fund">Sinking Fund</option>
                  <option value="one_off">One-Off</option>
                </select>
                <button type="submit" className="btn-ghost" disabled={saving || monthClosed || hasErrors(allocationErrors)}>
                  <Plus size={13} />
                  Add
                </button>
              </form>
            </section>

            <section className="wallet-panel wallet-capture-panel">
              <div className="wallet-panel-header">
                <div>
                  <span className="wallet-section-label">Quick Capture</span>
                  <strong>Transaction</strong>
                </div>
                <ReceiptText size={16} />
              </div>
              <TransactionCaptureForm
                summary={summary}
                saving={saving}
                monthClosed={monthClosed}
                activeAllocations={activeAllocations}
                quickCategoryOptions={quickCategoryOptions}
                transactionForm={transactionForm}
                setTransactionForm={setTransactionForm}
                transactionDisplayErrors={transactionDisplayErrors}
                transactionErrors={transactionErrors}
                transactionMoreOpen={transactionMoreOpen}
                setTransactionMoreOpen={setTransactionMoreOpen}
                touchTransaction={touchTransaction}
                submitTransaction={event => submitTransaction(event, 'desktop')}
                recentChips={recentTransactionChips}
                onRecentChipSelect={chip => selectRecentTransactionChip(chip, 'desktop')}
                amountInputRef={transactionAmountInputRef}
              />
            </section>

            <section className="wallet-panel wallet-income-panel">
              <div className="wallet-panel-header">
                <div>
                  <span className="wallet-section-label">Income</span>
                  <strong>{formatMoney(summary.income_total_cents)}</strong>
                </div>
              </div>
              <div className="wallet-income-list">
                {summary.income_items.length === 0 ? (
                  <div className="wallet-empty-row">No income recorded.</div>
                ) : summary.income_items.map(item => {
                  const editingIncome = incomeEdit?.id === item.id
                  return editingIncome ? (
                    <form key={item.id} onSubmit={saveIncomeEdit} className="wallet-income-edit-form">
                      <WalletField error={incomeEditDisplayErrors.name}>
                        <input
                          value={incomeEdit.name}
                          onChange={event => updateIncomeEdit({ name: event.target.value })}
                          onBlur={() => touchIncomeEdit('name')}
                          aria-label="Income name"
                          {...withFieldError(incomeEditDisplayErrors.name)}
                        />
                      </WalletField>
                      <WalletField error={incomeEditDisplayErrors.amount}>
                        <input
                          value={incomeEdit.amount}
                          onChange={event => updateIncomeEdit({ amount: event.target.value })}
                          onBlur={() => touchIncomeEdit('amount')}
                          inputMode="decimal"
                          aria-label="Income amount"
                          {...withFieldError(incomeEditDisplayErrors.amount)}
                        />
                      </WalletField>
                      <WalletField error={incomeEditDisplayErrors.receivedDate}>
                        <input
                          type="date"
                          value={incomeEdit.receivedDate}
                          onChange={event => updateIncomeEdit({ receivedDate: event.target.value })}
                          onBlur={() => touchIncomeEdit('receivedDate')}
                          aria-label="Income received date"
                          {...withFieldError(incomeEditDisplayErrors.receivedDate)}
                        />
                      </WalletField>
                      <input
                        value={incomeEdit.notes}
                        onChange={event => updateIncomeEdit({ notes: event.target.value })}
                        placeholder="Notes"
                        aria-label="Income notes"
                      />
                      <div className="wallet-income-edit-actions">
                        <button type="button" className="btn-ghost" onClick={() => setIncomeEdit(null)} disabled={saving}>Cancel</button>
                        <button type="submit" className="btn-primary" disabled={saving || monthClosed || hasErrors(incomeEditErrors)}>
                          <Save size={13} />
                          Save
                        </button>
                      </div>
                    </form>
                  ) : (
                    <div key={item.id} className="wallet-income-row">
                      <div>
                        <strong>{item.name}</strong>
                        <span>{item.received_date || item.applies_to_month}{item.notes ? ` / ${item.notes}` : ''}</span>
                      </div>
                      <strong>{formatMoney(item.amount_cents)}</strong>
                      <button
                        type="button"
                        className="btn-ghost wallet-row-action-btn"
                        onClick={() => startIncomeEdit(item)}
                        disabled={saving || monthClosed}
                        title="Edit income"
                        aria-label="Edit income"
                      >
                        <Pencil size={15} />
                      </button>
                    </div>
                  )
                })}
              </div>
              <form onSubmit={submitIncome} className="wallet-income-form">
                <WalletField error={incomeDisplayErrors.name}>
                  <input
                    value={incomeForm.name}
                    onChange={event => setIncomeForm(prev => ({ ...prev, name: event.target.value }))}
                    onBlur={() => touchIncome('name')}
                    placeholder="Income name"
                    {...withFieldError(incomeDisplayErrors.name)}
                  />
                </WalletField>
                <WalletField error={incomeDisplayErrors.amount}>
                  <input
                    value={incomeForm.amount}
                    onChange={event => setIncomeForm(prev => ({ ...prev, amount: event.target.value }))}
                    onBlur={() => touchIncome('amount')}
                    placeholder="Amount"
                    inputMode="decimal"
                    {...withFieldError(incomeDisplayErrors.amount)}
                  />
                </WalletField>
                <WalletField error={incomeDisplayErrors.receivedDate}>
                  <input
                    type="date"
                    value={incomeForm.receivedDate}
                    onChange={event => setIncomeForm(prev => ({ ...prev, receivedDate: event.target.value }))}
                    onBlur={() => touchIncome('receivedDate')}
                    {...withFieldError(incomeDisplayErrors.receivedDate)}
                  />
                </WalletField>
                <input
                  value={incomeForm.notes}
                  onChange={event => setIncomeForm(prev => ({ ...prev, notes: event.target.value }))}
                  placeholder="Notes"
                />
                <button type="submit" className="btn-ghost" disabled={saving || monthClosed || hasErrors(incomeErrors)}>
                  <Plus size={13} />
                  Add Income
                </button>
              </form>
            </section>
          </div>

          <RecentTransactionsPanel
            transactions={summary.recent_transactions || []}
            saving={saving}
            monthClosed={monthClosed}
            transactionAmountEdit={transactionAmountEdit}
            transactionAmountEditDisplayErrors={transactionAmountEditDisplayErrors}
            transactionAmountEditErrors={transactionAmountEditErrors}
            startTransactionAmountEdit={startTransactionAmountEdit}
            updateTransactionAmountEdit={updateTransactionAmountEdit}
            setTransactionAmountEdit={setTransactionAmountEdit}
            saveTransactionAmountEdit={saveTransactionAmountEdit}
            openSplitModal={openSplitModal}
            openSplitDetail={openSplitDetail}
            deleteTransaction={deleteTransaction}
          />
        </>
      )}
      {showCaptureFab && (
        <button
          type="button"
          className="wallet-mobile-capture-fab"
          onClick={() => setCaptureSheetOpen(true)}
          disabled={saving || monthClosed}
          aria-label="Capture transaction"
        >
          <ReceiptText size={17} />
          <span>Capture</span>
        </button>
      )}
      {summary && (
        <WalletMobileCaptureSheet
          open={captureSheetOpen}
          onClose={() => setCaptureSheetOpen(false)}
        >
          <TransactionCaptureForm
            summary={summary}
            saving={saving}
            monthClosed={monthClosed}
            activeAllocations={activeAllocations}
            quickCategoryOptions={quickCategoryOptions}
            transactionForm={transactionForm}
            setTransactionForm={setTransactionForm}
            transactionDisplayErrors={transactionDisplayErrors}
            transactionErrors={transactionErrors}
            transactionMoreOpen={transactionMoreOpen}
            setTransactionMoreOpen={setTransactionMoreOpen}
            touchTransaction={touchTransaction}
            submitTransaction={event => submitTransaction(event, 'sheet')}
            recentChips={recentTransactionChips}
            onRecentChipSelect={chip => selectRecentTransactionChip(chip, 'sheet')}
            amountInputRef={sheetTransactionAmountInputRef}
            variant="sheet"
          />
        </WalletMobileCaptureSheet>
      )}
      {reconcileOpen && summary && (
        <WalletModal title="Reconcile Balance" onClose={() => setReconcileOpen(false)}>
          <form onSubmit={submitReconciliation} className="wallet-reconcile-form">
            <div className="wallet-reconcile-grid">
              <SummaryMetric label="Previous Wallet" value={summary.wallet_balance_cents} />
              <SummaryMetric label="New Wallet" value={parsedBalanceInputCents || 0} />
              <SummaryMetric label="Expected" value={summary.expected_balance_cents} />
              <SummaryMetric label="Adjustment" value={balanceDifference || 0} tone={(balanceDifference || 0) === 0 ? 'good' : 'warn'} />
            </div>
            <label>
              <span>Reason</span>
              <select value={adjustmentReason} onChange={event => setAdjustmentReason(event.target.value)}>
                <option value="rounding">Rounding Adjustment</option>
                <option value="missed_transaction">Missed Transaction</option>
                <option value="cash_variance">Cash Variance</option>
                <option value="manual_correction">Manual Correction</option>
              </select>
            </label>
            <label>
              <span>Adjustment Note</span>
              <input value={adjustmentNote} onChange={event => setAdjustmentNote(event.target.value)} placeholder="Optional" />
            </label>
            <div className="wallet-modal-actions">
              <button type="button" className="btn-ghost" onClick={() => setReconcileOpen(false)}>Cancel</button>
              <button type="submit" className="btn-primary" disabled={saving || !balanceDifference || hasErrors(balanceErrors)}>
                <Save size={14} />
                Save Adjustment
              </button>
            </div>
          </form>
        </WalletModal>
      )}
      {splitTransaction && (
        <SplitTransactionModal
          transaction={splitTransaction}
          summary={summary}
          rows={splitRows}
          setRows={setSplitRows}
          saving={saving}
          onClose={() => setSplitTransaction(null)}
          onSubmit={submitSplit}
        />
      )}
      {splitDetail && (
        <SplitGroupModal
          detail={splitDetail}
          onClose={() => setSplitDetail(null)}
        />
      )}
      {monthBookOpen && (
        <MonthBookModal
          rows={monthBookRows}
          currentMonth={monthKey}
          saving={saving}
          summaryMonth={bookSummaryMonth}
          editForm={bookEditForm}
          setEditForm={setBookEditForm}
          onClose={() => {
            setMonthBookOpen(false)
            setBookEditForm(null)
          }}
          onView={viewBookMonth}
          onReports={reportBookMonth}
          onSummary={summarizeBookMonth}
          onEdit={editBookMonth}
          onSaveEdit={saveBookMonthEdit}
          onDelete={requestDeleteBookMonth}
          onReopen={reopenBookMonth}
        />
      )}
      {bookDeleteTarget && (
        <MonthDeleteConfirmModal
          target={bookDeleteTarget}
          value={bookDeleteConfirm}
          setValue={setBookDeleteConfirm}
          saving={saving}
          onClose={() => {
            setBookDeleteTarget(null)
            setBookDeleteConfirm('')
          }}
          onConfirm={confirmDeleteBookMonth}
        />
      )}
    </div>
  )
}
