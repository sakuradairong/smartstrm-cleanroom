import { useCallback, useState } from 'react'
import type { ThemeConfig } from 'antd'
import { theme as antdTheme } from 'antd'

export type ThemeMode = 'dark' | 'light'

const STORAGE_KEY = 'smartstrm.theme'

function readInitial(): ThemeMode {
  return localStorage.getItem(STORAGE_KEY) === 'light' ? 'light' : 'dark'
}

export function useTheme(): { mode: ThemeMode; toggle: () => void } {
  const [mode, setMode] = useState<ThemeMode>(readInitial)
  const toggle = useCallback(() => {
    setMode(current => {
      const next = current === 'dark' ? 'light' : 'dark'
      localStorage.setItem(STORAGE_KEY, next)
      return next
    })
  }, [])
  return { mode, toggle }
}

export function getAntdThemeConfig(mode: ThemeMode): ThemeConfig {
  const dark = mode === 'dark'
  const colors = dark
    ? {
        primary: '#2563eb',
        primaryText: '#60a5fa',
        layout: '#0d1015',
        container: '#181d27',
        elevated: '#181d27',
        subtle: '#161b24',
        border: '#242c3a',
        controlBorder: '#64748b',
        text: '#f1f5f9',
        secondary: '#94a3b8',
        success: '#4ade80',
        error: '#f87171',
        hover: '#1d2430',
      }
    : {
        primary: '#1d5bd6',
        primaryText: '#1d5bd6',
        layout: '#f4f6f9',
        container: '#ffffff',
        elevated: '#ffffff',
        subtle: '#f8fafc',
        border: '#e2e8f0',
        controlBorder: '#8492a6',
        text: '#0f172a',
        secondary: '#475569',
        success: '#15803d',
        error: '#dc2626',
        hover: '#f1f5f9',
      }

  return {
    algorithm: dark ? antdTheme.darkAlgorithm : antdTheme.defaultAlgorithm,
    token: {
      colorPrimary: colors.primary,
      colorInfo: colors.primary,
      colorLink: colors.primaryText,
      colorSuccess: colors.success,
      colorError: colors.error,
      colorBgLayout: colors.layout,
      colorBgContainer: colors.container,
      colorBgElevated: colors.elevated,
      colorBorder: colors.controlBorder,
      colorBorderSecondary: colors.border,
      colorText: colors.text,
      colorTextSecondary: colors.secondary,
      borderRadius: 8,
      borderRadiusLG: 10,
      controlHeight: 36,
      fontSize: 14,
      fontFamily:
        'system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Noto Sans SC", sans-serif',
      boxShadowTertiary: dark
        ? '0 12px 28px -20px rgb(0 0 0 / 76%)'
        : '0 10px 28px -22px rgb(15 23 42 / 24%)',
    },
    components: {
      Layout: {
        bodyBg: 'transparent',
        siderBg: 'transparent',
        footerBg: 'transparent',
        headerBg: 'transparent',
      },
      Menu: {
        itemHeight: 40,
        itemBorderRadius: 8,
        itemMarginInline: 10,
        itemSelectedBg: dark ? 'rgb(96 165 250 / 14%)' : '#e9f1ff',
        itemSelectedColor: colors.primaryText,
        itemHoverBg: colors.hover,
        itemHoverColor: colors.text,
        activeBarBorderWidth: 0,
        iconSize: 16,
        darkItemBg: 'transparent',
        darkItemSelectedBg: 'rgb(96 165 250 / 14%)',
        darkItemSelectedColor: '#60a5fa',
        darkItemHoverBg: colors.hover,
        darkItemHoverColor: colors.text,
      },
      Button: {
        fontWeight: 550,
        primaryShadow: 'none',
        defaultShadow: 'none',
        dangerShadow: 'none',
      },
      Card: {
        headerBg: 'transparent',
        bodyPadding: 20,
        headerHeight: 48,
      },
      Table: {
        headerBg: colors.subtle,
        headerColor: colors.secondary,
        headerSortActiveBg: colors.hover,
        headerSortHoverBg: colors.hover,
        rowHoverBg: colors.hover,
        borderColor: colors.border,
        cellPaddingBlock: 12,
        cellPaddingInline: 16,
      },
      Tabs: {
        itemSelectedColor: colors.primaryText,
        itemHoverColor: colors.primaryText,
        inkBarColor: colors.primaryText,
      },
      Input: {
        activeBorderColor: colors.primary,
        hoverBorderColor: colors.primary,
        activeShadow: `0 0 0 2px ${dark ? 'rgb(96 165 250 / 18%)' : 'rgb(29 91 214 / 14%)'}`,
      },
      Select: {
        activeBorderColor: colors.primary,
        hoverBorderColor: colors.primary,
        optionActiveBg: colors.hover,
        optionSelectedBg: dark ? 'rgb(96 165 250 / 14%)' : '#e9f1ff',
      },
      Modal: {
        contentBg: colors.elevated,
        headerBg: colors.elevated,
      },
      Drawer: {
        colorBgElevated: colors.elevated,
      },
    },
  }
}
