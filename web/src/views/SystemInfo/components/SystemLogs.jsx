import { useCallback, useEffect, useState } from 'react'
import { showError } from 'utils/common'

import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableContainer from '@mui/material/TableContainer'
import TableCell from '@mui/material/TableCell'
import TableRow from '@mui/material/TableRow'
import PerfectScrollbar from 'react-perfect-scrollbar'
import LinearProgress from '@mui/material/LinearProgress'
import Toolbar from '@mui/material/Toolbar'
import {
  Alert,
  Box,
  Button,
  Card,
  Chip,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControlLabel,
  IconButton,
  InputAdornment,
  Paper,
  Stack,
  Switch,
  TextField,
  Tooltip,
  Typography,
  useTheme
} from '@mui/material'
import Label from 'ui-component/Label'
import { useTranslation } from 'react-i18next'
import { Icon } from '@iconify/react'
import { API } from 'utils/api'

// System Logs Component
const SystemLogs = () => {
  const { t } = useTranslation()
  const theme = useTheme()

  // Cache screen size to avoid repeated calculations
  const [isMobile, setIsMobile] = useState(window.innerWidth < 600)
  const [autoRefresh, setAutoRefresh] = useState(false)
  const [refreshInterval, setRefreshInterval] = useState(5000)
  const [maxEntries, setMaxEntries] = useState(50)
  const [logs, setLogs] = useState([])
  const [originalLogs, setOriginalLogs] = useState([]) // Store original complete log data
  const [searchTerm, setSearchTerm] = useState('')
  const [isInitialized, setIsInitialized] = useState(false)
  const [previousLogCount, setPreviousLogCount] = useState(0) // Track log count for auto-scroll
  const [lastLogTimestamp, setLastLogTimestamp] = useState(null) // Track latest log timestamp for auto-scroll
  const [showScrollToBottom, setShowScrollToBottom] = useState(false) // Show scroll to bottom button
  const [userScrolledUp, setUserScrolledUp] = useState(false) // Track if user manually scrolled up
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  // Search options
  const [useRegex, setUseRegex] = useState(false)

  // Context dialog state
  const [contextDialogOpen, setContextDialogOpen] = useState(false)
  const [contextLogs, setContextLogs] = useState([])
  const [contextLoading, setContextLoading] = useState(false)
  const [selectedLogIndex, setSelectedLogIndex] = useState(-1)
  const [contextLines, setContextLines] = useState(3)
  const [targetLogPosition, setTargetLogPosition] = useState(-1)

  // Get scroll containers for log table
  const getScrollContainers = () => {
    return [
      document.querySelector('.log-table-scroll .ps'),
      document.querySelector('.log-table-scroll'),
      document.querySelector('.MuiTableContainer-root')
    ].filter(Boolean)
  }

  // Update last log timestamp for auto-scroll detection
  const updateLastLogTimestamp = (logs) => {
    if (logs.length > 0) {
      setLastLogTimestamp(logs[logs.length - 1].timestamp)
    }
  }

  // Process a log entry from the backend
  const processLogEntry = (entry) => {
    try {
      const level = entry.Level.toLowerCase()
      const typeMap = {
        'error': 'error', 'err': 'error', 'fatal': 'error',
        'warn': 'warning', 'warning': 'warning',
        'debug': 'debug',
        'info': 'info'
      }

      return {
        timestamp: new Date(entry.Timestamp).toISOString(),
        type: typeMap[level] || 'info',
        message: entry.Message
      }
    } catch (error) {
      console.error('Error processing log entry:', error, entry)
      return {
        timestamp: new Date().toISOString(),
        type: 'error',
        message: `Failed to process log entry: ${JSON.stringify(entry)}`
      }
    }
  }

  // Fetch logs using search query
  const fetchSearchLogs = useCallback(async(isAutoRefresh = false) => {
    if (!isAutoRefresh) {
      setLoading(true)
    }
    setError('')

    try {
      // Fetch complete logs and search results in parallel for better performance
      const [fullLogsResponse, searchResponse] = await Promise.all([
        API.post('/api/system_info/log', { count: maxEntries }),
        API.post('/api/system_info/log/query', {
          count: maxEntries,
          search_term: searchTerm,
          use_regex: useRegex
        })
      ])

      // Process complete logs for context lookup
      if (fullLogsResponse.data.success) {
        const fullLogData = fullLogsResponse.data.data
        const processedFullLogs = fullLogData.map((entry, index) => processLogEntry(entry, index))
        setOriginalLogs(processedFullLogs)
      } else {
        console.warn('Failed to fetch complete logs for context lookup')
      }

      const response = searchResponse

      if (response.data.success) {
        const result = response.data.data
        if (result && result.logs && Array.isArray(result.logs)) {
          const processedLogs = result.logs.map(processLogEntry)
          setLogs(processedLogs)
          updateLastLogTimestamp(processedLogs)
        } else {
          setLogs([])
        }
      } else {
        setError('Failed to fetch logs: ' + response.data.message)
        showError(response.data.message)
      }
    } catch (error) {
      setError('Error fetching logs: ' + error.message)
      showError(error.message)
    } finally {
      if (!isAutoRefresh) {
        setLoading(false)
      }
    }
  }, [maxEntries, searchTerm, useRegex])

  // Fetch logs from the API
  const fetchLogs = useCallback(async(isAutoRefresh = false) => {
    // If there's a search term, use search API, otherwise use regular API
    if (searchTerm.trim()) {
      return fetchSearchLogs(isAutoRefresh)
    }

    // Only show loading indicator for manual refresh to avoid page jumping
    if (!isAutoRefresh) {
      setLoading(true)
    }
    setError('')

    try {
      const response = await API.post('/api/system_info/log', {
        count: maxEntries
      })

      if (response.data.success) {
        const logData = response.data.data
        const processedLogs = logData.map(processLogEntry)
        setLogs(processedLogs)
        setOriginalLogs(processedLogs)
        updateLastLogTimestamp(processedLogs)
      } else {
        setError('Failed to fetch logs: ' + response.data.message)
        if (!isAutoRefresh) {
          showError(response.data.message)
        }
      }
    } catch (error) {
      setError('Error fetching logs: ' + error.message)
      if (!isAutoRefresh) {
        showError(error.message)
      }
    } finally {
      if (!isAutoRefresh) {
        setLoading(false)
      }
    }
  }, [maxEntries, searchTerm, fetchSearchLogs])



  // Initialize logs on component mount
  useEffect(() => {
    if (!isInitialized) {
      fetchLogs()
      setIsInitialized(true)
    }
  }, [fetchLogs, isInitialized])

  // Refetch logs when maxEntries changes (but not during initialization)
  useEffect(() => {
    if (isInitialized) {
      fetchLogs()
    }
  }, [maxEntries, fetchLogs, isInitialized])

  // Set up auto-refresh interval
  useEffect(() => {
    // Only set up interval if autoRefresh is enabled
    let interval
    if (autoRefresh && isInitialized) {
      interval = setInterval(() => {
        fetchLogs(true).then(() => {
          // Only auto-scroll if user hasn't manually scrolled up
          if (!userScrolledUp) {
            setTimeout(() => {
              const containers = getScrollContainers()
              for (const container of containers) {
                if (container.scrollHeight > container.clientHeight &&
                    container.querySelector('tbody')) {
                  container.scrollTop = container.scrollHeight
                  break
                }
              }
            }, 200)
          }
        })
      }, refreshInterval)
    }

    return () => {
      if (interval) clearInterval(interval)
    }
  }, [autoRefresh, fetchLogs, refreshInterval, isInitialized, userScrolledUp])

  // Auto-scroll to bottom when NEW logs are added during auto-refresh
  useEffect(() => {
    // Only auto-scroll if:
    // 1. Auto-refresh is enabled
    // 2. We have logs
    // 3. User hasn't manually scrolled up
    if (autoRefresh && logs.length > 0 && !userScrolledUp) {
      // Check if we have new logs by comparing timestamps or count
      const hasNewLogs = logs.length > previousLogCount ||
                        (lastLogTimestamp && logs.length > 0 &&
                         logs[logs.length - 1].timestamp !== lastLogTimestamp)

      if (hasNewLogs) {
        const containers = getScrollContainers()
        for (const container of containers) {
          if (container.scrollHeight > container.clientHeight &&
              container.querySelector('tbody')) {
            setTimeout(() => {
              container.scrollTop = container.scrollHeight
            }, 100)
            break
          }
        }
      }
    }

    // Update previous log count for next comparison
    setPreviousLogCount(logs.length)
  }, [logs, autoRefresh, previousLogCount, lastLogTimestamp, userScrolledUp])

  // Monitor window size changes
  useEffect(() => {
    const handleResize = () => {
      setIsMobile(window.innerWidth < 600)
    }

    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

  // Monitor scroll position to show/hide scroll to bottom button
  useEffect(() => {
    const containers = getScrollContainers()
    let scrollTimeout

    const handleScroll = () => {
      clearTimeout(scrollTimeout)
      scrollTimeout = setTimeout(checkScrollPosition, 100)
    }

    containers.forEach(container => {
      container.addEventListener('scroll', handleScroll)
    })

    setTimeout(checkScrollPosition, 300)

    return () => {
      clearTimeout(scrollTimeout)
      containers.forEach(container => {
        container.removeEventListener('scroll', handleScroll)
      })
    }
  }, [logs])

  // Handle max entries change
  const handleMaxEntriesChange = (event) => {
    const value = parseInt(event.target.value)
    if (!isNaN(value) && value > 0 && value <= 999) {
      setMaxEntries(value)
    }
  }

  // Handle search term change
  const handleSearchChange = (event) => {
    setSearchTerm(event.target.value)
  }

  // Clear logs display
  const handleClearLogs = () => {
    setLogs([])
    setOriginalLogs([]) // Clear original log data to free memory
  }

  // Clear search
  const handleClearSearch = () => {
    setSearchTerm('')
  }



  // Check if user is at bottom of scroll container
  const checkScrollPosition = () => {
    if (logs.length === 0) {
      setShowScrollToBottom(false)
      setUserScrolledUp(false)
      return
    }

    const containers = getScrollContainers()
    for (const container of containers) {
      if (container.scrollHeight > container.clientHeight &&
          container.querySelector('tbody')) {
        const isAtBottom = container.scrollTop >= container.scrollHeight - container.clientHeight - 50
        setShowScrollToBottom(!isAtBottom)
        setUserScrolledUp(!isAtBottom)
        break
      }
    }
  }

  // Scroll to bottom manually
  const scrollToBottom = () => {
    const containers = getScrollContainers()
    for (const container of containers) {
      if (container.scrollHeight > container.clientHeight &&
          container.querySelector('tbody')) {
        container.scrollTop = container.scrollHeight
        setShowScrollToBottom(false)
        setUserScrolledUp(false)
        break
      }
    }
  }

  // Auto-trigger search when search term changes
  useEffect(() => {
    if (searchTerm.trim() && isInitialized) {
      const timeoutId = setTimeout(() => {
        fetchSearchLogs()
      }, 500) // Debounce search

      return () => clearTimeout(timeoutId)
    }
  }, [searchTerm, useRegex, isInitialized, fetchSearchLogs])

  // Reset advanced filters
  const handleResetFilters = () => {
    setUseRegex(false)
    setSearchTerm('')
    setError('')
  }

  // Get context for a specific log entry
  const handleGetContext = (logIndex) => {
    let targetLog, realLogIndex

    if (searchTerm.trim()) {
      // Search mode: get target log from currently displayed logs
      targetLog = logs[logIndex]

      // Strategy 1: Use content matching for search results (most reliable)
      realLogIndex = originalLogs.findIndex(log =>
        log.timestamp === targetLog.timestamp &&
        log.message === targetLog.message &&
        log.type === targetLog.type
      )

      // Strategy 2: If not found, try fuzzy matching by message only
      if (realLogIndex === -1) {
        realLogIndex = originalLogs.findIndex(log =>
          log.message === targetLog.message
        )
      }

      // Strategy 3: If still not found, use backend API
      if (realLogIndex === -1) {
        console.warn('Target log not found in originalLogs, using backend API')
        getContextFromBackend(targetLog)
        return
      }
    } else {
      // No search state: use index directly
      realLogIndex = logIndex
    }
    setSelectedLogIndex(realLogIndex)
    setContextDialogOpen(true)
    getContextLogs(realLogIndex)
  }

  // Get context from backend API when frontend lookup fails
  const getContextFromBackend = async (targetLog) => {
    setContextLoading(true)
    setSelectedLogIndex(-1) // Indicate backend mode
    setContextDialogOpen(true)

    try {
      const response = await API.post('/api/system_info/log/context', {
        timestamp: targetLog.timestamp,
        message: targetLog.message,
        type: targetLog.type,
        context_lines: contextLines
      })

      if (response.data.success && response.data.data) {
        const result = response.data.data
        if (result.logs && Array.isArray(result.logs)) {
          const processedLogs = result.logs.map(processLogEntry)
          setContextLogs(processedLogs)
          setTargetLogPosition(result.target_position || Math.floor(processedLogs.length / 2))
        } else {
          setContextLogs([targetLog]) // At least show the target log
          setTargetLogPosition(0)
        }
      } else {
        console.error('Backend context API failed:', response.data.message)
        setContextLogs([targetLog])
        setTargetLogPosition(0)
      }
    } catch (error) {
      console.error('Error fetching context from backend:', error)
      setContextLogs([targetLog])
      setTargetLogPosition(0)
    } finally {
      setContextLoading(false)
    }
  }

  // Get context logs directly from frontend data (much simpler!)
  const getContextLogs = (logIndex, lines = contextLines) => {
    setContextLoading(true)

    try {
      // Select data source based on current mode
      let sourceData
      if (searchTerm.trim() && originalLogs.length > 0) {
        // Search mode: use original complete log data
        sourceData = originalLogs
      } else {
        // Normal mode: use current log data
        sourceData = logs
      }

      // Simple frontend calculation - no need for backend API!
      const start = Math.max(0, logIndex - lines)
      const end = Math.min(sourceData.length, logIndex + lines + 1)
      const contextLogs = sourceData.slice(start, end)

      setContextLogs(contextLogs)

      // Target position in the context array
      const targetPosition = logIndex - start
      setTargetLogPosition(targetPosition)

    } catch (error) {
      showError('Error getting context: ' + error.message)
    } finally {
      setContextLoading(false)
    }
  }

  // Close context dialog
  const handleCloseContextDialog = () => {
    setContextDialogOpen(false)
    setContextLogs([])
    setSelectedLogIndex(-1)
    setTargetLogPosition(-1)
  }



  // Get log label color for display (using original project's Label component colors)
  const getLogLabelColor = (type) => {
    switch (type.toLowerCase()) {
      case 'error':
        return 'error'
      case 'warn':
      case 'warning':
        return 'warning'
      case 'info':
        return 'info'
      case 'debug':
        return 'secondary'
      default:
        return 'default'
    }
  }

  // Format timestamp
  const formatTimestamp = (timestamp) => {
    const date = new Date(timestamp)
    return date.toLocaleDateString() + ' ' + date.toLocaleTimeString()
  }

  return (
    <>
      <Card sx={{ position: 'relative' }}>
        <Toolbar
          sx={{
            pl: { sm: 2 },
            pr: { xs: 1, sm: 1 },
            flexDirection: { xs: 'column', sm: 'row' },
            alignItems: { xs: 'stretch', sm: 'center' },
            gap: { xs: 1, sm: 0 },
            py: { xs: 2, sm: 1 }
          }}
        >
          <Typography
            variant="h6"
            component="div"
            sx={{
              whiteSpace: 'nowrap',
              textAlign: { xs: 'center', sm: 'left' },
              mb: { xs: 1, sm: 0 }
            }}
          >
            {t('System Logs')}
          </Typography>

          <Stack
            direction={{ xs: 'column', sm: 'row' }}
            spacing={1}
            alignItems="center"
            sx={{
              ml: { sm: 'auto' },
              width: { xs: '100%', sm: 'auto' }
            }}
          >
            <Stack
              direction="row"
              spacing={1}
              alignItems="center"
              sx={{
                justifyContent: { xs: 'space-between', sm: 'flex-start' },
                width: { xs: '100%', sm: 'auto' }
              }}
            >
              <FormControlLabel
                control={<Switch checked={autoRefresh} onChange={(e) => setAutoRefresh(e.target.checked)} size="small"/>}
                label={t('Auto Refresh')}
                sx={{
                  minWidth: 'auto',
                  '& .MuiFormControlLabel-label': {
                    fontSize: { xs: '0.875rem', sm: '1rem' }
                  }
                }}
              />
              <TextField
                label="Period"
                type="number"
                size="small"
                value={refreshInterval / 1000}
                onChange={(e) => {
                  const seconds = parseInt(e.target.value) || 1
                  const clampedSeconds = Math.min(Math.max(seconds, 1), 60)
                  setRefreshInterval(clampedSeconds * 1000)
                }}
                InputProps={{
                  inputProps: { min: 1, max: 60 },
                  endAdornment: <InputAdornment position="end">s</InputAdornment>
                }}
                sx={{ width: { xs: 60, sm: 70 } }}
              />
              <TextField
                label="Limit"
                type="number"
                size="small"
                value={maxEntries}
                onChange={handleMaxEntriesChange}
                InputProps={{ inputProps: { min: 1, max: 999 } }}
                sx={{ width: { xs: 60, sm: 70 } }}
              />
              <IconButton onClick={handleClearLogs} color="error" size="small">
                <Icon icon="solar:trash-bin-trash-bold"/>
              </IconButton>
            </Stack>
          </Stack>
        </Toolbar>
        {/* Error Alert */}
        {error && (
          <Alert severity="error" sx={{ mb: 2 }} onClose={() => setError('')}>
            {error}
          </Alert>
        )}

        {/* Toolbar */}
        <Toolbar sx={{ pl: 0, pr: 0, minHeight: '48px !important', flexWrap: 'wrap', gap: 1 }}>
          <Stack
            direction={{ xs: 'column', md: 'row' }}
            spacing={1}
            alignItems={{ xs: 'stretch', md: 'center' }}
            sx={{ width: '100%' }}
          >
            <TextField
              size="small"
              placeholder={useRegex ? 'Enter regular expression...' : t('Search logs...')}
              value={searchTerm}
              onChange={handleSearchChange}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <Icon icon={useRegex ? 'solar:code-bold' : 'solar:magnifer-linear'}/>
                  </InputAdornment>
                ),
                endAdornment: searchTerm && (
                  <InputAdornment position="end">
                    <IconButton size="small" onClick={handleClearSearch}>
                      <Icon icon="solar:close-circle-bold"/>
                    </IconButton>
                  </InputAdornment>
                )
              }}
              sx={{ flexGrow: 1, maxWidth: { xs: '100%', md: 400 } }}
            />

            <Stack direction="row" spacing={1} alignItems="center" sx={{ flexWrap: 'wrap' }}>
              <FormControlLabel
                control={
                  <Switch
                    checked={useRegex}
                    onChange={(e) => setUseRegex(e.target.checked)}
                    size="small"
                  />
                }
                label="Regex"
              />

              <Typography variant="body2" color="textSecondary" sx={{ whiteSpace: 'nowrap' }}>
                {searchTerm ?
                  `${logs.length} results` :
                  `${logs.length} logs`
                }
              </Typography>
            </Stack>
          </Stack>
        </Toolbar>

        {loading && (
          <LinearProgress
            sx={{
              position: 'absolute',
              top: 0,
              left: 0,
              right: 0,
              zIndex: 1,
              height: '2px'
            }}
          />
        )}

        <Box sx={{ height: 600, maxHeight: '70vh', overflow: 'hidden', position: 'relative' }}>
          {/* Scroll to bottom button */}
          {showScrollToBottom && (
            <Box
              sx={{
                position: 'absolute',
                bottom: 20,
                left: '50%',
                transform: 'translateX(-50%)',
                zIndex: 10,
                display: 'flex',
                justifyContent: 'center'
              }}
            >
              <IconButton
                onClick={scrollToBottom}
                sx={{
                  backgroundColor: theme.palette.mode === 'dark'
                    ? 'rgba(66, 66, 66, 0.95)'
                    : 'rgba(255, 255, 255, 0.95)',
                  border: `1px solid ${theme.palette.divider}`,
                  boxShadow: theme.shadows[4],
                  width: 40,
                  height: 40,
                  backdropFilter: 'blur(8px)',
                  '&:hover': {
                    backgroundColor: theme.palette.mode === 'dark'
                      ? 'rgba(88, 88, 88, 1)'
                      : 'rgba(248, 249, 250, 1)',
                    boxShadow: theme.shadows[8]
                  },
                  transition: 'all 0.2s ease-in-out'
                }}
              >
                <Icon
                  icon="solar:arrow-down-bold"
                  style={{
                    fontSize: '20px',
                    color: theme.palette.text.secondary
                  }}
                />
              </IconButton>
            </Box>
          )}

          <PerfectScrollbar component="div" style={{ height: '100%' }} className="log-table-scroll">
            <TableContainer sx={{ height: '100%' }}>
              <Table stickyHeader>
                <TableBody>
                  {logs.length === 0 && !loading ? (
                    <TableRow>
                      <TableCell colSpan={4} align="center" sx={{ py: 3 }}>
                        <Typography variant="body2" color="textSecondary">
                          {t('No logs available')}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ) : logs.length === 0 && !loading ? (
                    <TableRow>
                      <TableCell colSpan={4} align="center" sx={{ py: 3 }}>
                        <Typography variant="body2" color="textSecondary">
                          {t('No matching logs found')}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ) : (
                    logs.map((log, index) => (
                      <TableRow key={index} hover>
                        <TableCell sx={{ width: 180, py: 1 }}>
                          <Typography
                            variant="caption"
                            color="textSecondary"
                            sx={{
                              fontFamily: 'monospace',
                              fontSize: '11px'
                            }}
                          >
                            {formatTimestamp(log.timestamp)}
                          </Typography>
                        </TableCell>

                        <TableCell sx={{ width: 80, py: 1 }}>
                          <Label
                            variant="filled"
                            color={getLogLabelColor(log.type)}
                          >
                            {log.type.toUpperCase()}
                          </Label>
                        </TableCell>

                        <TableCell sx={{ py: 1, textAlign: 'left' }}>
                          <Typography
                            variant="body2"
                            sx={{
                              fontFamily: 'monospace',
                              fontSize: '12px',
                              lineHeight: 1.4,
                              wordWrap: 'break-word',
                              whiteSpace: 'pre-wrap',
                              maxWidth: 'none',
                              textAlign: 'left'
                            }}
                          >
                            {log.message}
                          </Typography>
                        </TableCell>

                        <TableCell sx={{ width: 60, py: 1 }}>
                          <Tooltip title={t('View Context')} arrow>
                            <IconButton
                              size="small"
                              onClick={() => handleGetContext(index)}
                            >
                              <Icon icon="mdi:unfold-more-horizontal"/>
                            </IconButton>
                          </Tooltip>
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
            </TableContainer>
          </PerfectScrollbar>

        </Box>
      </Card>

      {/* Context Dialog */}
      <Dialog
        open={contextDialogOpen}
        onClose={handleCloseContextDialog}
        maxWidth="lg"
        fullWidth
        fullScreen={isMobile}
        PaperProps={{
          sx: { height: isMobile ? '100vh' : '80vh' }
        }}
      >
        <DialogTitle>
          <Stack direction="row" alignItems="center" spacing={2} sx={{ flexWrap: 'wrap' }}>
            <Icon icon="mdi:unfold-more-horizontal"/>
            <Typography variant="h6">Log Context</Typography>
            {selectedLogIndex >= 0 && (
              <Chip
                label={`Target: Log #${selectedLogIndex + 1}`}
                size="small"
                color="primary"
              />
            )}
            <TextField
              label="Context Lines"
              type="number"
              size="small"
              value={contextLines}
              onChange={(e) => {
                const value = e.target.value === '' ? '' : parseInt(e.target.value)
                if (value === '' || (value >= 1 && value <= 20)) {
                  const finalValue = value === '' ? 1 : value
                  setContextLines(finalValue)
                  if (selectedLogIndex >= 0 && value !== '') {
                    getContextLogs(selectedLogIndex, finalValue)
                  }
                }
              }}
              onBlur={(e) => {
                const inputValue = e.target.value
                if (inputValue === '' || parseInt(inputValue) < 1) {
                  setContextLines(1)
                  if (selectedLogIndex >= 0) {
                    getContextLogs(selectedLogIndex, 1)
                  }
                }
              }}
              InputProps={{
                inputProps: { min: 1, max: 20 }
              }}
              sx={{ width: 120 }}
            />
          </Stack>
        </DialogTitle>
        <DialogContent dividers>
          {contextLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
              <Icon icon="solar:loading-bold" style={{ fontSize: '2rem', animation: 'spin 1s linear infinite' }}/>
            </Box>
          ) : (
            <PerfectScrollbar component="div">
              <TableContainer component={Paper} variant="outlined">
                <Table size="small" sx={{ '& .MuiTableCell-root': { textAlign: 'left' } }}>
                  <TableBody>
                    {contextLogs.map((log, index) => {
                      const isTargetLog = index === targetLogPosition

                      return (
                        <TableRow
                          key={index}
                          sx={{
                            backgroundColor: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? 'rgba(255, 193, 7, 0.12)'
                                : 'rgba(0, 122, 255, 0.04)'
                              : 'transparent',
                            border: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? '1px solid rgba(255, 193, 7, 0.3)'
                                : '1px solid rgba(0, 122, 255, 0.12)'
                              : theme.palette.mode === 'dark'
                                ? '1px solid rgba(255, 255, 255, 0.12)'
                                : '1px solid rgba(0, 0, 0, 0.06)',
                            minHeight: '50px',
                            boxShadow: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? '0 1px 6px rgba(255, 193, 7, 0.2)'
                                : '0 1px 6px rgba(0, 122, 255, 0.08)'
                              : 'none',
                            borderRadius: isTargetLog ? '6px' : '0',
                            transform: 'none',
                            transition: 'all 0.15s ease-out',
                            position: 'relative',
                            borderLeft: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? '3px solid rgba(255, 193, 7, 0.8)'
                                : '3px solid rgba(0, 122, 255, 0.4)'
                              : 'none'
                          }}
                        >
                          <TableCell sx={{
                            padding: '8px 4px',
                            width: isMobile ? '120px' : '180px',
                            fontFamily: 'monospace',
                            fontSize: isMobile ? '10px' : '11px',
                            verticalAlign: 'top',
                            lineHeight: 1.2,
                            color: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? 'rgba(255, 193, 7, 1)'
                                : 'rgba(0, 122, 255, 0.7)'
                              : 'inherit',
                            fontWeight: isTargetLog ? '450' : 'normal'
                          }}>
                            {formatTimestamp(log.timestamp)}
                          </TableCell>
                          <TableCell sx={{
                            padding: '8px 4px',
                            width: isMobile ? '60px' : '80px',
                            verticalAlign: 'top'
                          }}>
                            <Label
                              variant="filled"
                              color={getLogLabelColor(log.type)}
                              sx={{ fontSize: isMobile ? '10px' : '12px' }}
                            >
                              {isMobile ? log.type.charAt(0) : log.type.toUpperCase()}
                            </Label>
                          </TableCell>
                          <TableCell sx={{
                            padding: '8px 4px',
                            fontFamily: 'monospace',
                            fontSize: isMobile ? '11px' : '13px',
                            wordBreak: 'break-word',
                            verticalAlign: 'top',
                            lineHeight: 1.4,
                            whiteSpace: 'pre-wrap',
                            maxWidth: isMobile ? '200px' : '500px',
                            color: isTargetLog
                              ? theme.palette.mode === 'dark'
                                ? 'rgba(255, 255, 255, 1)'
                                : 'rgba(0, 0, 0, 0.85)'
                              : 'inherit',
                            fontWeight: isTargetLog ? '450' : 'normal'
                          }}>
                            {log.message}
                          </TableCell>
                        </TableRow>
                      )
                    })}
                  </TableBody>
                </Table>
              </TableContainer>
            </PerfectScrollbar>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseContextDialog} color="primary">
            Close
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default SystemLogs
