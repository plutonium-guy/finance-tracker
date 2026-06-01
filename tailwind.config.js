/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: 'class',
  content: [
    './internal/web/templates/**/*.html',
    './internal/web/**/*.go', // classes built in Go (e.g. amountClass)
  ],
  theme: { extend: {} },
  plugins: [],
}
