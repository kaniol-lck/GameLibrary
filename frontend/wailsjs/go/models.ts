export namespace config {
	
	export class MetadataSource {
	    key: string;
	    name: string;
	    enabled: boolean;
	    settings?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new MetadataSource(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.enabled = source["enabled"];
	        this.settings = source["settings"];
	    }
	}
	export class Config {
	    schemaVersion: number;
	    machineId: string;
	    gameDirectories: string[];
	    gameDirectoryLabels?: Record<string, Array<string>>;
	    maxScanDepth: number;
	    language: string;
	    steamUserId?: string;
	    watcherEnabled: boolean;
	    watcherDebounceMs: number;
	    scrapeConcurrency?: number;
	    logLevel?: string;
	    logToLibrary?: boolean;
	    metadataSources: MetadataSource[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.machineId = source["machineId"];
	        this.gameDirectories = source["gameDirectories"];
	        this.gameDirectoryLabels = source["gameDirectoryLabels"];
	        this.maxScanDepth = source["maxScanDepth"];
	        this.language = source["language"];
	        this.steamUserId = source["steamUserId"];
	        this.watcherEnabled = source["watcherEnabled"];
	        this.watcherDebounceMs = source["watcherDebounceMs"];
	        this.scrapeConcurrency = source["scrapeConcurrency"];
	        this.logLevel = source["logLevel"];
	        this.logToLibrary = source["logToLibrary"];
	        this.metadataSources = this.convertValues(source["metadataSources"], MetadataSource);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace game {
	
	export class Executable {
	    path: string;
	    name: string;
	    primary: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Executable(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.primary = source["primary"];
	    }
	}
	export class Metadata {
	    coverUrl?: string;
	    coverLandscape?: string;
	    releaseDate?: string;
	    developer?: string;
	    publisher?: string;
	    tags?: string[];
	    description?: string;
	    links?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new Metadata(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.coverUrl = source["coverUrl"];
	        this.coverLandscape = source["coverLandscape"];
	        this.releaseDate = source["releaseDate"];
	        this.developer = source["developer"];
	        this.publisher = source["publisher"];
	        this.tags = source["tags"];
	        this.description = source["description"];
	        this.links = source["links"];
	    }
	}
	export class SavePath {
	    type: string;
	    path: string;
	    source?: string;
	
	    static createFrom(source: any = {}) {
	        return new SavePath(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.path = source["path"];
	        this.source = source["source"];
	    }
	}
	export class PlatformInfo {
	    platform: string;
	    id?: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new PlatformInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.platform = source["platform"];
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}
	export class GameInfo {
	    schemaVersion: number;
	    id: string;
	    title: string;
	    titleNative?: string;
	    type: string;
	    platforms?: PlatformInfo[];
	    aliases?: string[];
	    preferredSource?: string;
	    executables: Executable[];
	    savePaths?: SavePath[];
	    metadata?: Metadata;
	    scannedAt: string;
	    totalPlaytime: number;
	    lastPlayedAt?: string;
	    starred?: boolean;
	    tags?: string[];
	    coverVersion?: number;
	
	    static createFrom(source: any = {}) {
	        return new GameInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.schemaVersion = source["schemaVersion"];
	        this.id = source["id"];
	        this.title = source["title"];
	        this.titleNative = source["titleNative"];
	        this.type = source["type"];
	        this.platforms = this.convertValues(source["platforms"], PlatformInfo);
	        this.aliases = source["aliases"];
	        this.preferredSource = source["preferredSource"];
	        this.executables = this.convertValues(source["executables"], Executable);
	        this.savePaths = this.convertValues(source["savePaths"], SavePath);
	        this.metadata = this.convertValues(source["metadata"], Metadata);
	        this.scannedAt = source["scannedAt"];
	        this.totalPlaytime = source["totalPlaytime"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	        this.starred = source["starred"];
	        this.tags = source["tags"];
	        this.coverVersion = source["coverVersion"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	

}

export namespace main {
	
	export class AppInfo {
	    exeDir: string;
	    machineId: string;
	    machineName: string;
	    version: string;
	    buildTime: string;
	    logDir: string;
	    platform: string;
	    coverBaseUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new AppInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.exeDir = source["exeDir"];
	        this.machineId = source["machineId"];
	        this.machineName = source["machineName"];
	        this.version = source["version"];
	        this.buildTime = source["buildTime"];
	        this.logDir = source["logDir"];
	        this.platform = source["platform"];
	        this.coverBaseUrl = source["coverBaseUrl"];
	    }
	}
	export class ScrapeReport {
	    gameId: string;
	    title: string;
	    source?: string;
	    sources?: string[];
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScrapeReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	        this.source = source["source"];
	        this.sources = source["sources"];
	        this.error = source["error"];
	    }
	}
	export class SteamUserInfo {
	    id: string;
	    name?: string;
	
	    static createFrom(source: any = {}) {
	        return new SteamUserInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	    }
	}

}

export namespace scanner {
	
	export class ScanResult {
	    gameDir: string;
	    gameInfo?: game.GameInfo;
	    isNew: boolean;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameDir = source["gameDir"];
	        this.gameInfo = this.convertValues(source["gameInfo"], game.GameInfo);
	        this.isNew = source["isNew"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace taskqueue {
	
	export class Failure {
	    gameId: string;
	    title?: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new Failure(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	        this.error = source["error"];
	    }
	}
	export class Status {
	    pending: number;
	    running: number;
	    paused: boolean;
	    concurrency: number;
	    currentTitle?: string;
	    currentGameId?: string;
	    runningTitles: string[];
	    pendingTitles: string[];
	    completed: number;
	    failed: number;
	    total: number;
	    failures: Failure[];
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.pending = source["pending"];
	        this.running = source["running"];
	        this.paused = source["paused"];
	        this.concurrency = source["concurrency"];
	        this.currentTitle = source["currentTitle"];
	        this.currentGameId = source["currentGameId"];
	        this.runningTitles = source["runningTitles"];
	        this.pendingTitles = source["pendingTitles"];
	        this.completed = source["completed"];
	        this.failed = source["failed"];
	        this.total = source["total"];
	        this.failures = this.convertValues(source["failures"], Failure);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

