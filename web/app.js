// Bookmarchive Web Client
class BookmarchiveClient {
    constructor() {
        this.searchInput = document.getElementById('search-input');
        this.searchForm = document.getElementById('search-form');
        this.accountFilter = document.getElementById('account-filter');
        this.resultsContainer = document.getElementById('results-container');
        this.searchStatus = document.getElementById('search-status');
        this.loadingIndicator = document.getElementById('loading-indicator');
        this.totalCount = document.getElementById('total-count');
        this.resultsCount = document.getElementById('results-count');
        this.connectionStatus = document.getElementById('connection-status');
        this.activityStatus = document.getElementById('activity-status');
        this.lastUpdate = document.getElementById('last-update');

        this.searchTimeout = null;
        this.currentQuery = '';
        this.isSearching = false;
        this.eventSource = null;

        this.init();
    }

    init() {
        this.setupEventListeners();
        this.setupServerSentEvents();
        this.setupKeyboardShortcuts();
        this.loadInitialStats();
        this.loadRecentBookmarks(); // Load recent bookmarks on startup
        
        // Focus search input on load
        this.searchInput.focus();
    }

    setupEventListeners() {
        // Search form submission
        this.searchForm.addEventListener('submit', (e) => {
            e.preventDefault();
            this.performSearch();
        });

        // Real-time search on input
        this.searchInput.addEventListener('input', () => {
            this.handleSearchInput();
        });

        // Clear search on escape
        this.searchInput.addEventListener('keydown', (e) => {
            if (e.key === 'Escape') {
                this.clearSearch();
            }
        });

        // Account filter change
        this.accountFilter.addEventListener('change', () => {
            this.handleFilterChange();
        });

        // Handle result navigation with arrow keys
        document.addEventListener('keydown', (e) => {
            if (e.target === this.searchInput) return;
            
            const results = document.querySelectorAll('.result-card[tabindex="0"]');
            const currentIndex = Array.from(results).findIndex(el => el === document.activeElement);
            
            if (e.key === 'ArrowDown' && currentIndex < results.length - 1) {
                e.preventDefault();
                results[currentIndex + 1]?.focus();
            } else if (e.key === 'ArrowUp' && currentIndex > 0) {
                e.preventDefault();
                results[currentIndex - 1]?.focus();
            }
        });
    }

    setupServerSentEvents() {
        this.connectToEventStream();
    }

    connectToEventStream() {
        if (this.eventSource) {
            this.eventSource.close();
        }

        console.log('Connecting to event stream...');
        this.eventSource = new EventSource('/api/events');
        
        this.eventSource.onopen = () => {
            this.updateConnectionStatus('connected');
            this.updateActivityStatus('Ready');
            console.log('Connected to event stream');
        };

        this.eventSource.onmessage = (event) => {
            try {
                const data = JSON.parse(event.data);
                this.handleServerEvent(data);
            } catch (error) {
                console.error('Error parsing server event:', error);
            }
        };

        this.eventSource.onerror = (error) => {
            console.error('EventSource error:', error);
            this.updateConnectionStatus('reconnecting');
            this.updateActivityStatus('Reconnecting...');
            
            // Close the current connection
            if (this.eventSource) {
                this.eventSource.close();
                this.eventSource = null;
            }
            
            // Attempt to reconnect after 2 seconds with exponential backoff
            const reconnectDelay = Math.min(5000, 1000 * Math.pow(2, (this.reconnectAttempts || 0)));
            this.reconnectAttempts = (this.reconnectAttempts || 0) + 1;
            
            setTimeout(() => {
                if (!this.eventSource || this.eventSource.readyState === EventSource.CLOSED) {
                    this.connectToEventStream();
                }
            }, reconnectDelay);
        };

        // Reset reconnect attempts on successful connection
        this.eventSource.addEventListener('connected', () => {
            this.reconnectAttempts = 0;
        });
    }

    handleServerEvent(data) {
        console.log('Received event:', data); // Debug logging
        
        switch (data.type) {
            case 'connected':
                this.updateConnectionStatus('connected');
                this.updateActivityStatus('Ready');
                // Reset reconnect attempts on successful connection
                this.reconnectAttempts = 0;
                break;
            case 'heartbeat':
                // Just acknowledge the heartbeat, don't log it
                break;
            case 'stats':
                this.updateStats(data.payload);
                break;
            case 'backfill_start':
                this.updateActivityStatus('Processing');
                break;
            case 'backfill_complete':
                this.updateActivityStatus('Processing');
                // Refresh stats and recent bookmarks to show updated content
                setTimeout(() => {
                    this.loadInitialStats();
                    this.updateActivityStatus('Ready');
                    // Reload recent bookmarks if no search is active
                    if (!this.searchInput.value.trim()) {
                        this.loadRecentBookmarks();
                    }
                }, 2000);
                break;
            case 'polling_status':
                this.updatePollingStatus(data.payload);
                break;
            case 'activity':
                // Only pass through simple state messages
                const message = data.payload.message;
                if (message === 'Ready' || message === 'Processing' || message === 'Error') {
                    this.updateActivityStatus(message);
                }
                break;
            case 'batch_start':
                this.updateActivityStatus('Processing');
                break;
            case 'bookmark_processed':
                // Don't update status for individual bookmark processing
                // Keep status as "Processing" until batch is complete
                break;
            case 'batch_complete':
                this.updateActivityStatus('Processing');
                // Refresh stats and recent bookmarks to show updated content
                setTimeout(() => {
                    this.loadInitialStats();
                    this.updateActivityStatus('Ready');
                    // Reload recent bookmarks if no search is active
                    if (!this.searchInput.value.trim()) {
                        this.loadRecentBookmarks();
                    }
                }, 1000);
                break;
            default:
                console.log('Unknown event type:', data.type);
        }
    }

    setupKeyboardShortcuts() {
        document.addEventListener('keydown', (e) => {
            // Focus search with '/' key (like many search interfaces)
            if (e.key === '/' && document.activeElement !== this.searchInput) {
                e.preventDefault();
                this.searchInput.focus();
                this.searchInput.select();
            }
        });
    }

    handleSearchInput() {
        const query = this.searchInput.value.trim();
        
        // Clear previous timeout
        if (this.searchTimeout) {
            clearTimeout(this.searchTimeout);
        }

        // Debounce search requests
        this.searchTimeout = setTimeout(() => {
            if (query !== this.currentQuery) {
                this.currentQuery = query;
                this.performSearch();
            }
        }, 300);
    }

    handleFilterChange() {
        const query = this.searchInput.value.trim();
        
        if (query) {
            // If there's a search query, perform search with new filter
            this.performSearch();
        } else {
            // If no search query, reload recent bookmarks with new filter
            this.loadRecentBookmarks();
        }
    }

    async performSearch() {
        const query = this.searchInput.value.trim();
        
        if (!query) {
            this.loadRecentBookmarks();
            return;
        }

        if (this.isSearching) {
            return; // Prevent concurrent searches
        }

        this.isSearching = true;
        this.showLoading();

        try {
            const response = await fetch('/api/search', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    query: query,
                    limit: 50,
                    offset: 0,
                    enable_highlighting: true,
                    snippet_length: 200,
                    filter_by_account: this.accountFilter.value
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const results = await response.json();
            this.displayResults(results, query);
            
        } catch (error) {
            console.error('Search error:', error);
            this.updateActivityStatus('Error');
            this.showError('Search failed. Please try again.');
        } finally {
            this.isSearching = false;
            this.hideLoading();
        }
    }

    displayResults(results, query) {
        if (!results || results.length === 0) {
            this.showNoResults(query);
            return;
        }

        this.hideSearchStatus();
        this.resultsContainer.hidden = false;
        this.resultsContainer.innerHTML = '';
        
        results.forEach((result, index) => {
            const resultElement = this.createResultElement(result, index);
            this.resultsContainer.appendChild(resultElement);
        });

        this.updateResultsCount(results.length);
        
        // Announce results to screen readers
        this.announceToScreenReader(`Found ${results.length} results for "${query}"`);
    }

    displayRecentBookmarks(results) {
        if (!results || results.length === 0) {
            this.showEmptyState();
            return;
        }

        this.hideSearchStatus();
        this.resultsContainer.hidden = false;
        this.resultsContainer.innerHTML = '';
        
        results.forEach((result, index) => {
            const resultElement = this.createResultElement(result, index, true);
            this.resultsContainer.appendChild(resultElement);
        });

        this.updateResultsCount(results.length);
        
        // Announce to screen readers
        this.announceToScreenReader(`Showing ${results.length} recent bookmarks`);
    }

    showEmptyState() {
        this.resultsContainer.hidden = true;
        this.searchStatus.innerHTML = `
            <div class="empty-state">
                <h2>No bookmarks yet</h2>
                <p>Your bookmarks will appear here as they're imported from Mastodon.</p>
                <p><small>Start typing above to search once you have bookmarks.</small></p>
            </div>
        `;
        this.searchStatus.hidden = false;
        this.updateResultsCount(0);
    }

    createResultElement(result, index, isRecentBookmark = false) {
        const bookmark = result.bookmark;
        const card = document.createElement('article');
        card.className = 'result-card';
        if (isRecentBookmark) {
            card.classList.add('recent-bookmark');
        }
        card.setAttribute('tabindex', '0');
        card.setAttribute('role', 'article');
        card.setAttribute('aria-label', `${isRecentBookmark ? 'Recent bookmark' : 'Search result'} ${index + 1}`);

        // Parse the raw_json to get complete bookmark data
        let fullBookmark = null;
        let statusData = {};
        try {
            const parsed = JSON.parse(bookmark.raw_json);
            if (parsed.status) {
                // New format with complete bookmark data
                fullBookmark = parsed;
                statusData = parsed.status;
            } else {
                // Old format with minimal data
                statusData = parsed;
            }
        } catch (e) {
            // Fallback if JSON parsing fails
            statusData = { id: bookmark.status_id };
        }

        // Extract account information
        let username = 'unknown';
        let displayName = 'Mastodon User';
        let avatar = 'https://mastodon.social/avatars/original/missing.png';
        let statusUrl = `https://mastodon.social/@unknown/${bookmark.status_id}`;

        if (fullBookmark && fullBookmark.status && fullBookmark.status.account) {
            const account = fullBookmark.status.account;
            username = account.username || 'unknown';
            displayName = account.display_name || account.username || 'Mastodon User';
            avatar = account.avatar || avatar;
            
            // Try to construct proper URL from the status data
            if (fullBookmark.status.url) {
                statusUrl = fullBookmark.status.url;
            } else if (fullBookmark.status.uri) {
                statusUrl = fullBookmark.status.uri;
            } else {
                // Fallback: construct URL with proper username
                statusUrl = `https://mastodon.social/@${username}/${bookmark.status_id}`;
            }
        } else {
            // Try to extract username from search_text for old bookmarks
            const searchText = bookmark.search_text || '';
            const matches = searchText.match(/@(\w+)/);
            if (matches && matches[1]) {
                username = matches[1];
                statusUrl = `https://mastodon.social/@${username}/${bookmark.status_id}`;
            }
        }

        // Use snippet if available, otherwise use search_text or status content
        let content = result.snippet;
        if (!content) {
            if (fullBookmark && fullBookmark.status && fullBookmark.status.content) {
                content = fullBookmark.status.content;
            } else {
                content = bookmark.search_text || '';
            }
        }
        
        // For recent bookmarks, show bookmarked_at instead of created_at for date
        const dateToShow = isRecentBookmark ? bookmark.bookmarked_at : bookmark.created_at;
        
        card.innerHTML = `
            <header class="result-header">
                <img src="${this.escapeHTML(avatar)}" 
                     alt="Avatar for ${this.escapeHTML(displayName)}" 
                     class="result-avatar"
                     loading="lazy"
                     onerror="this.src='https://mastodon.social/avatars/original/missing.png'">
                <div class="result-author">
                    <a href="https://mastodon.social/@${this.escapeHTML(username)}" 
                       class="result-author-name" 
                       target="_blank" 
                       rel="noopener noreferrer">
                        ${this.escapeHTML(displayName)}
                    </a>
                    <div class="result-author-handle">@${this.escapeHTML(username)}</div>
                </div>
                <time class="result-date" 
                      datetime="${dateToShow}"
                      title="${isRecentBookmark ? 'Bookmarked' : 'Posted'} ${this.formatDate(dateToShow)}">
                    ${this.formatDate(dateToShow)}
                </time>
            </header>
            
            <div class="result-content">
                <div class="result-snippet">${this.sanitizeHTML(content)}</div>
            </div>
            
            <footer class="result-footer">
                ${!isRecentBookmark ? `
                    <div class="result-score">
                        Score: ${result.rank ? result.rank.toFixed(2) : 'N/A'}
                    </div>
                ` : `
                    <div class="result-type">
                        Recent Bookmark
                    </div>
                `}
                <div class="result-actions">
                    <a href="${this.escapeHTML(statusUrl)}" 
                       target="_blank" 
                       rel="noopener noreferrer" 
                       class="result-link">
                        View Post
                    </a>
                </div>
            </footer>
        `;

        // Add enter key handler for keyboard accessibility
        card.addEventListener('keydown', (e) => {
            if (e.key === 'Enter') {
                e.preventDefault();
                const link = card.querySelector('.result-link');
                if (link) {
                    link.click();
                }
            }
        });

        return card;
    }

    showNoResults(query) {
        this.resultsContainer.hidden = true;
        this.searchStatus.innerHTML = `
            <p>No bookmarks found for "<strong>${this.escapeHTML(query)}</strong>"</p>
            <p><small>Try different keywords or check your spelling</small></p>
        `;
        this.searchStatus.hidden = false;
        this.updateResultsCount(0);
        
        this.announceToScreenReader(`No results found for "${query}"`);
    }

    showError(message) {
        this.resultsContainer.hidden = true;
        this.searchStatus.innerHTML = `
            <p style="color: #e53e3e;">⚠️ ${this.escapeHTML(message)}</p>
        `;
        this.searchStatus.hidden = false;
        this.updateResultsCount(0);
    }

    clearSearch() {
        this.searchInput.value = '';
        this.currentQuery = '';
        this.clearResults();
        this.searchInput.focus();
    }

    clearResults() {
        this.resultsContainer.hidden = true;
        this.resultsContainer.innerHTML = '';
        // Load recent bookmarks instead of showing empty search prompt
        this.loadRecentBookmarks();
        this.updateResultsCount('--');
    }

    showLoading() {
        this.loadingIndicator.hidden = false;
        this.loadingIndicator.setAttribute('aria-hidden', 'false');
    }

    hideLoading() {
        this.loadingIndicator.hidden = true;
        this.loadingIndicator.setAttribute('aria-hidden', 'true');
    }

    hideSearchStatus() {
        this.searchStatus.hidden = true;
    }

    updateConnectionStatus(status) {
        this.connectionStatus.className = `status-indicator ${status}`;
        
        const statusText = {
            connected: 'Connected',
            connecting: 'Connecting...',
            disconnected: 'Disconnected',
            reconnecting: 'Reconnecting...'
        };
        
        this.connectionStatus.title = statusText[status] || status;
        
        // Also update the text content for better accessibility
        this.connectionStatus.setAttribute('aria-label', statusText[status] || status);
    }

    updateActivityStatus(message) {
        this.activityStatus.textContent = message;
        this.lastUpdate.textContent = new Date().toLocaleTimeString();
    }

    updateStats(stats) {
        this.totalCount.textContent = stats.total_bookmarks || '--';
        this.lastUpdate.textContent = new Date().toLocaleTimeString();
    }

    updateResultsCount(count) {
        this.resultsCount.textContent = count;
    }

    updatePollingStatus(status) {
        if (status.active) {
            this.updateActivityStatus('Processing');
        } else {
            this.updateActivityStatus('Ready');
        }
    }

    async loadInitialStats() {
        try {
            const response = await fetch('/api/stats');
            if (response.ok) {
                const stats = await response.json();
                this.updateStats(stats);
            }
        } catch (error) {
            console.error('Failed to load initial stats:', error);
        }
    }

    async loadRecentBookmarks() {
        // Don't load recent bookmarks if user is already typing
        if (this.searchInput.value.trim()) {
            return;
        }

        try {
            this.showLoading();
            
            const response = await fetch('/api/search', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    query: '', // Empty query to get recent bookmarks
                    limit: 20,
                    offset: 0,
                    enable_highlighting: false,
                    snippet_length: 200,
                    filter_by_account: this.accountFilter.value
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP error! status: ${response.status}`);
            }

            const results = await response.json();
            
            if (results && results.length > 0) {
                this.displayRecentBookmarks(results);
            } else {
                this.showEmptyState();
            }
            
        } catch (error) {
            console.error('Failed to load recent bookmarks:', error);
            this.showEmptyState();
        } finally {
            this.hideLoading();
        }
    }

    // Utility functions
    formatDate(dateString) {
        try {
            const date = new Date(dateString);
            const now = new Date();
            const diffMs = now - date;
            const diffHours = diffMs / (1000 * 60 * 60);
            
            if (diffHours < 1) {
                const diffMins = Math.floor(diffMs / (1000 * 60));
                return `${diffMins}m ago`;
            } else if (diffHours < 24) {
                return `${Math.floor(diffHours)}h ago`;
            } else {
                const diffDays = Math.floor(diffHours / 24);
                if (diffDays < 7) {
                    return `${diffDays}d ago`;
                } else {
                    return date.toLocaleDateString();
                }
            }
        } catch (error) {
            return 'Unknown';
        }
    }

    sanitizeHTML(html) {
        // Since the content comes from Mastodon API and is already sanitized,
        // we can trust most of the HTML. We just need to handle search highlighting
        // and basic safety measures
        
        if (!html) return '';
        
        // For content from Mastodon API, preserve the HTML as-is
        // The API already sanitizes dangerous content
        let content = html;
        
        // Only escape content if it looks like it contains unescaped dangerous elements
        // Check for script tags, event handlers, or javascript: protocols
        const hasDangerousContent = /<script[^>]*>/i.test(content) ||
                                   /on\w+\s*=/i.test(content) ||
                                   /javascript:/i.test(content);
        
        if (hasDangerousContent) {
            // If we detect potentially dangerous content, escape it completely
            const div = document.createElement('div');
            div.textContent = content;
            content = div.innerHTML;
            
            // Then allow back only the safe highlighting tags
            content = content.replace(/&lt;mark&gt;/g, '<mark>');
            content = content.replace(/&lt;\/mark&gt;/g, '</mark>');
        }
        
        return content;
    }

    escapeHTML(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    announceToScreenReader(message) {
        // Create a temporary element for screen reader announcements
        const announcement = document.createElement('div');
        announcement.setAttribute('aria-live', 'polite');
        announcement.setAttribute('aria-atomic', 'true');
        announcement.className = 'visually-hidden';
        announcement.textContent = message;
        
        document.body.appendChild(announcement);
        
        // Remove after a short delay
        setTimeout(() => {
            document.body.removeChild(announcement);
        }, 1000);
    }

    // Cleanup method
    destroy() {
        if (this.eventSource) {
            this.eventSource.close();
        }
        
        if (this.searchTimeout) {
            clearTimeout(this.searchTimeout);
        }
    }
}

// Initialize the application when DOM is loaded
document.addEventListener('DOMContentLoaded', () => {
    window.bookmarchiveClient = new BookmarchiveClient();
});

// Cleanup on page unload
window.addEventListener('beforeunload', () => {
    if (window.bookmarchiveClient) {
        window.bookmarchiveClient.destroy();
    }
});
