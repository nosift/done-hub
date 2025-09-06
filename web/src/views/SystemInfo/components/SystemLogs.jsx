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
  FormControl,
  FormControlLabel,
  IconButton,
  InputAdornment,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  Stack,
  Switch,
  TextField,
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
  const [logIndexMap, setLogIndexMap] = useState(new Map()) // Log index cache for performance optimization
  const [searchTerm, setSearchTerm] = useState('')
  const [filteredLogs, setFilteredLogs] = useState([])
  const [isInitialized, setIsInitialized] = useState(false)
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

  // Create index map for fast log lookup
  const createLogIndexMap = useCallback((logs) => {
    const indexMap = new Map()
    logs.forEach((log, index) => {
      // Use timestamp + message combination as unique key to avoid duplicates
      const key = `${log.timestamp}-${log.message.substring(0, 100)}`
      indexMap.set(key, index)
    })
    setLogIndexMap(indexMap)
  }, [])

  // Process a log entry from the backend
  const processLogEntry = (entry) => {
    try {
      // Map log levels to our frontend types
      let type = entry.Level.toLowerCase()

      // Map log levels to our frontend types
      switch (type) {
        case 'info':
          type = 'info'
          break
        case 'error':
        case 'err':
        case 'fatal':
          type = 'error'
          break
        case 'warn':
        case 'warning':
          type = 'warning'
          break
        case 'debug':
          type = 'debug'
          break
        default:
          type = 'info'
      }

      return {
        timestamp: new Date(entry.Timestamp).toISOString(),
        type,
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
  const fetchSearchLogs = useCallback(async() => {
    setLoading(true)
    setError('')

    try {
      // Optimization: only fetch complete logs when truly needed, avoid duplicate calls
      let needFullLogs = originalLogs.length === 0

      if (needFullLogs) {
        const fullLogsResponse = await API.post('/api/system_info/log', {
          count: maxEntries
        })

        if (fullLogsResponse.data.success) {
          const fullLogData = fullLogsResponse.data.data
          const processedFullLogs = fullLogData.map(processLogEntry)
          setOriginalLogs(processedFullLogs)
          createLogIndexMap(processedFullLogs) // Create index cache
        }
      }

      const queryParams = {
        count: maxEntries,
        search_term: searchTerm,
        use_regex: useRegex
      }

      const response = await API.post('/api/system_info/log/query', queryParams)

      if (response.data.success) {
        const result = response.data.data
        if (result && result.logs && Array.isArray(result.logs)) {
          const processedLogs = result.logs.map(processLogEntry)
          setLogs(processedLogs)
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
      setLoading(false)
    }
  }, [maxEntries, searchTerm, useRegex])

  // Fetch logs from the API
  const fetchLogs = useCallback(async(isAutoRefresh = false) => {
    // If there's a search term, use search API, otherwise use regular API
    if (searchTerm.trim()) {
      return fetchSearchLogs()
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
        // Store original complete log data for context queries
        setOriginalLogs(processedLogs)
        // Create index cache for performance optimization
        createLogIndexMap(processedLogs)
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
      interval = setInterval(() => fetchLogs(true), refreshInterval)
    }

    return () => {
      if (interval) clearInterval(interval)
    }
  }, [autoRefresh, fetchLogs, refreshInterval, isInitialized])

  // Set filtered logs to current logs (no client-side filtering needed)
  useEffect(() => {
    setFilteredLogs(logs)
  }, [logs])

  // Auto-scroll to bottom when logs update during auto-refresh
  useEffect(() => {
    if (autoRefresh && logs.length > 0) {
      // Use specific selector to target only the log table scroll container
      const containers = [
        // Primary target: our specific log table scroll container
        document.querySelector('.log-table-scroll .ps'),
        document.querySelector('.log-table-scroll'),
        // Fallback: table container within the logs card
        document.querySelector('.MuiTableContainer-root')
      ].filter(Boolean)

      for (const container of containers) {
        // Only use containers that are actually scrollable and contain table rows
        if (container.scrollHeight > container.clientHeight &&
            container.querySelector('tbody')) {
          setTimeout(() => {
            container.scrollTop = container.scrollHeight - container.clientHeight
          }, 100)
          break
        }
      }
    }
  }, [logs, autoRefresh])

  // Monitor window size changes
  useEffect(() => {
    const handleResize = () => {
      setIsMobile(window.innerWidth < 600)
    }

    window.addEventListener('resize', handleResize)
    return () => window.removeEventListener('resize', handleResize)
  }, [])

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
    setLogIndexMap(new Map()) // Clear index cache
    setFilteredLogs([])
  }

  // Clear search
  const handleClearSearch = () => {
    setSearchTerm('')
  }

  // Handle search
  const handleSearch = () => {
    fetchLogs()
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
      // Use index cache for fast lookup, fallback to traditional method
      const key = `${targetLog.timestamp}-${targetLog.message.substring(0, 100)}`
      realLogIndex = logIndexMap.get(key)

      if (realLogIndex === undefined) {
        // Cache miss, use traditional search method
        realLogIndex = originalLogs.findIndex(log =>
          log.timestamp === targetLog.timestamp &&
          log.message === targetLog.message &&
          log.type === targetLog.type
        )
      }
    } else {
      // No search state: use index directly
      realLogIndex = logIndex
    }

    setSelectedLogIndex(realLogIndex)
    setContextDialogOpen(true)
    getContextLogs(realLogIndex)
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

  // Handle refresh interval change
  const handleRefreshIntervalChange = (event) => {
    const value = parseInt(event.target.value)
    setRefreshInterval(value)
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
            py: { xs: 1, sm: 0 }
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
                sx={{ width: { xs: 70, sm: 80 } }}
              />
              <TextField
                label="Limit"
                type="number"
                size="small"
                value={maxEntries}
                onChange={handleMaxEntriesChange}
                InputProps={{ inputProps: { min: 1, max: 999 } }}
                sx={{ width: { xs: 70, sm: 80 } }}
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
                  `${filteredLogs.length}/${logs.length}` :
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

        <Box sx={{ height: 600, maxHeight: '70vh', overflow: 'hidden' }}>
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
                  ) : filteredLogs.length === 0 && !loading ? (
                    <TableRow>
                      <TableCell colSpan={4} align="center" sx={{ py: 3 }}>
                        <Typography variant="body2" color="textSecondary">
                          {t('No matching logs found')}
                        </Typography>
                      </TableCell>
                    </TableRow>
                  ) : (
                    filteredLogs.map((log, index) => (
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
                          <IconButton
                            size="small"
                            onClick={() => handleGetContext(index)}
                            title="View Context"
                          >
                            <Icon icon="mdi:unfold-more-horizontal"/>
                          </IconButton>
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
