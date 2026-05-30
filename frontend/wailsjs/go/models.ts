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
	    machineId: string;
	    gameDirectories: string[];
	    gameDirectoryLabels?: Record<string, Array<string>>;
	    maxScanDepth: number;
	    language: string;
	    metadataSources: MetadataSource[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.machineId = source["machineId"];
	        this.gameDirectories = source["gameDirectories"];
	        this.gameDirectoryLabels = source["gameDirectoryLabels"];
	        this.maxScanDepth = source["maxScanDepth"];
	        this.language = source["language"];
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
	    id: string;
	    title: string;
	    titleNative?: string;
	    platforms?: PlatformInfo[];
	    aliases?: string[];
	    preferredSource?: string;
	    type: string;
	    executables: Executable[];
	    savePaths?: SavePath[];
	    metadata?: Metadata;
	    scannedAt: string;
	    totalPlaytime: number;
	    lastPlayedAt?: string;
	    starred?: boolean;
	    tags?: string[];
	
	    static createFrom(source: any = {}) {
	        return new GameInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.titleNative = source["titleNative"];
	        this.platforms = this.convertValues(source["platforms"], PlatformInfo);
	        this.aliases = source["aliases"];
	        this.preferredSource = source["preferredSource"];
	        this.type = source["type"];
	        this.executables = this.convertValues(source["executables"], Executable);
	        this.savePaths = this.convertValues(source["savePaths"], SavePath);
	        this.metadata = this.convertValues(source["metadata"], Metadata);
	        this.scannedAt = source["scannedAt"];
	        this.totalPlaytime = source["totalPlaytime"];
	        this.lastPlayedAt = source["lastPlayedAt"];
	        this.starred = source["starred"];
	        this.tags = source["tags"];
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
	
	export class ScrapeReport {
	    gameId: string;
	    title: string;
	    source: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new ScrapeReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.gameId = source["gameId"];
	        this.title = source["title"];
	        this.source = source["source"];
	        this.error = source["error"];
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

