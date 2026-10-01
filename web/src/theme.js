export function getTheme() {
  return localStorage.getItem('pagefire-theme') === 'dark' ? 'dark' : 'light'
}

export function setTheme(theme) {
  const value = theme === 'dark' ? 'dark' : 'light'
  localStorage.setItem('pagefire-theme', value)
  document.documentElement.dataset.theme = value
}
