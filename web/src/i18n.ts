import i18n from 'i18next'
import { initReactI18next } from 'react-i18next'
import enCommon from './locales/en/common.json'

// To add a language: create locales/<lang>/common.json with the same keys and
// register it here. Missing keys fall back to English.
export const resources = {
  en: { common: enCommon },
} as const

export type Language = keyof typeof resources

declare module 'i18next' {
  interface CustomTypeOptions {
    defaultNS: 'common'
    // Typed resources make `t('some.key')` a compile error if the key is not in en/common.json.
    resources: (typeof resources)['en']
  }
}

function detectLanguage(): Language {
  const supported = Object.keys(resources) as Language[]
  for (const tag of navigator.languages ?? []) {
    const base = tag.split('-')[0] as Language
    if (supported.includes(base)) return base
  }
  return 'en'
}

void i18n.use(initReactI18next).init({
  resources,
  lng: detectLanguage(),
  fallbackLng: 'en',
  defaultNS: 'common',
  interpolation: { escapeValue: false }, // React already escapes
})

export default i18n
