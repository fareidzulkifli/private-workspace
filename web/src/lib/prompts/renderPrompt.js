const variablePattern = /\{([a-zA-Z][a-zA-Z0-9_-]*)\}/g

const escapeRegExp = (value) => value.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')

export const extractVariables = (body = '') => {
  const variables = []
  const seen = new Set()

  for (const match of body.matchAll(variablePattern)) {
    const name = match[1]
    if (!seen.has(name)) {
      seen.add(name)
      variables.push(name)
    }
  }

  return variables
}

export const buildContextText = (items = [], enabledIds = null) =>
  items
    .filter(item => item?.body?.trim())
    .filter(item => !enabledIds || enabledIds.has(item.id))
    .sort((a, b) => (a.sort_order || 0) - (b.sort_order || 0))
    .map(item => `${item.title?.trim() || 'Context'}\n${item.body.trim()}`)
    .join('\n\n')
    .trim()

export const renderPrompt = ({ body = '', values = {}, contextText = '' } = {}) => {
  const variables = extractVariables(body)
  const hasContextVariable = variables.includes('context')
  const mergedValues = {
    ...values,
    ...(hasContextVariable && contextText ? { context: contextText } : {}),
  }

  let rendered = body

  variables.forEach(variable => {
    const pattern = new RegExp(`\\{${escapeRegExp(variable)}\\}`, 'g')
    rendered = rendered.replace(pattern, mergedValues[variable] || '')
  })

  if (contextText && !hasContextVariable) {
    rendered = `${rendered.trim()}\n\n---\nContext:\n${contextText}`
  }

  return rendered.trim()
}

export const estimateTokens = (text = '') => Math.max(1, Math.ceil(text.length / 4))
