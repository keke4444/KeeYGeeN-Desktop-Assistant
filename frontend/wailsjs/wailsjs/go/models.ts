export namespace main {
	
	export class CatInfo {
	    key: string;
	    name: string;
	    path: string;
	    count: number;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new CatInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.count = source["count"];
	        this.icon = source["icon"];
	    }
	}
	export class EntryInfo {
	    name: string;
	    path: string;
	    size: number;
	    sizeText: string;
	    ext: string;
	    kind: string;
	    modTime: string;
	    isDir: boolean;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new EntryInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.sizeText = source["sizeText"];
	        this.ext = source["ext"];
	        this.kind = source["kind"];
	        this.modTime = source["modTime"];
	        this.isDir = source["isDir"];
	        this.icon = source["icon"];
	    }
	}
	export class FontPartSetting {
	    family: string;
	    size: number;
	    color: string;
	
	    static createFrom(source: any = {}) {
	        return new FontPartSetting(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.family = source["family"];
	        this.size = source["size"];
	        this.color = source["color"];
	    }
	}
	export class ItemResult {
	    name: string;
	    src: string;
	    dst: string;
	    ok: boolean;
	    skipped: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new ItemResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.src = source["src"];
	        this.dst = source["dst"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.reason = source["reason"];
	    }
	}
	export class Listing {
	    path: string;
	    parent: string;
	    name: string;
	    isRoot: boolean;
	    isHome: boolean;
	    entries: EntryInfo[];
	
	    static createFrom(source: any = {}) {
	        return new Listing(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.parent = source["parent"];
	        this.name = source["name"];
	        this.isRoot = source["isRoot"];
	        this.isHome = source["isHome"];
	        this.entries = this.convertValues(source["entries"], EntryInfo);
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
	export class MoveReport {
	    total: number;
	    ok: number;
	    skipped: number;
	    failed: number;
	    batch: string;
	    target: string;
	    items: ItemResult[];
	
	    static createFrom(source: any = {}) {
	        return new MoveReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.total = source["total"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.batch = source["batch"];
	        this.target = source["target"];
	        this.items = this.convertValues(source["items"], ItemResult);
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
	export class OpenItem {
	    key: string;
	    name: string;
	    path: string;
	    icon?: string;
	
	    static createFrom(source: any = {}) {
	        return new OpenItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.key = source["key"];
	        this.name = source["name"];
	        this.path = source["path"];
	        this.icon = source["icon"];
	    }
	}
	export class RecentGroup {
	    id: string;
	    name: string;
	    count: number;
	
	    static createFrom(source: any = {}) {
	        return new RecentGroup(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.count = source["count"];
	    }
	}
	export class RecentItem {
	    path: string;
	    name: string;
	    ext: string;
	    kind: string;
	    icon?: string;
	    count: number;
	    lastUsed: number;
	    lastUsedText: string;
	    exists: boolean;
	    task: string;
	
	    static createFrom(source: any = {}) {
	        return new RecentItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.path = source["path"];
	        this.name = source["name"];
	        this.ext = source["ext"];
	        this.kind = source["kind"];
	        this.icon = source["icon"];
	        this.count = source["count"];
	        this.lastUsed = source["lastUsed"];
	        this.lastUsedText = source["lastUsedText"];
	        this.exists = source["exists"];
	        this.task = source["task"];
	    }
	}
	export class Settings {
	    bgImage: string;
	    bgName: string;
	    hasBackground: boolean;
	    bgOpacity: number;
	    bgFit: string;
	    bgScope: string;
	    bgRect: number[];
	    categoriesPosition: string;
	    iconScale: number;
	    hotkey: string;
	    themeAccent: string;
	    themePage: string;
	    themePageCustom: boolean;
	    themeBack: string;
	    followThemeBack: boolean;
	    themeRecent: string;
	    followRecent: boolean;
	    fontFamily: string;
	    fontSize: number;
	    fontColor: string;
	    fontParts: Record<string, FontPartSetting>;
	    searchCandidates: number;
	    candRuleFile: string;
	    candRuleName: string;
	    candRuleError?: string;
	    autoStart: boolean;
	    closeToTaskbar: boolean;
	    desktopPath: string;
	    homePath: string;
	    hotkeyError?: string;
	    autoStartError?: string;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.bgImage = source["bgImage"];
	        this.bgName = source["bgName"];
	        this.hasBackground = source["hasBackground"];
	        this.bgOpacity = source["bgOpacity"];
	        this.bgFit = source["bgFit"];
	        this.bgScope = source["bgScope"];
	        this.bgRect = source["bgRect"];
	        this.categoriesPosition = source["categoriesPosition"];
	        this.iconScale = source["iconScale"];
	        this.hotkey = source["hotkey"];
	        this.themeAccent = source["themeAccent"];
	        this.themePage = source["themePage"];
	        this.themePageCustom = source["themePageCustom"];
	        this.themeBack = source["themeBack"];
	        this.followThemeBack = source["followThemeBack"];
	        this.themeRecent = source["themeRecent"];
	        this.followRecent = source["followRecent"];
	        this.fontFamily = source["fontFamily"];
	        this.fontSize = source["fontSize"];
	        this.fontColor = source["fontColor"];
	        this.fontParts = this.convertValues(source["fontParts"], FontPartSetting, true);
	        this.searchCandidates = source["searchCandidates"];
	        this.candRuleFile = source["candRuleFile"];
	        this.candRuleName = source["candRuleName"];
	        this.candRuleError = source["candRuleError"];
	        this.autoStart = source["autoStart"];
	        this.closeToTaskbar = source["closeToTaskbar"];
	        this.desktopPath = source["desktopPath"];
	        this.homePath = source["homePath"];
	        this.hotkeyError = source["hotkeyError"];
	        this.autoStartError = source["autoStartError"];
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
	export class State {
	    desktop: string;
	    root: string;
	    desktopName: string;
	    desktopIcon?: string;
	    homeName: string;
	    homeIcon?: string;
	    openFolders: OpenItem[];
	    categories: CatInfo[];
	    sidebarWidth: number;
	    configPath: string;
	    journalPath: string;
	    version: string;
	    verify: string;
	    conflict: string;
	    loadError?: string;
	
	    static createFrom(source: any = {}) {
	        return new State(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.desktop = source["desktop"];
	        this.root = source["root"];
	        this.desktopName = source["desktopName"];
	        this.desktopIcon = source["desktopIcon"];
	        this.homeName = source["homeName"];
	        this.homeIcon = source["homeIcon"];
	        this.openFolders = this.convertValues(source["openFolders"], OpenItem);
	        this.categories = this.convertValues(source["categories"], CatInfo);
	        this.sidebarWidth = source["sidebarWidth"];
	        this.configPath = source["configPath"];
	        this.journalPath = source["journalPath"];
	        this.version = source["version"];
	        this.verify = source["verify"];
	        this.conflict = source["conflict"];
	        this.loadError = source["loadError"];
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
	export class UndoItem {
	    name: string;
	    ok: boolean;
	    reason?: string;
	
	    static createFrom(source: any = {}) {
	        return new UndoItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.ok = source["ok"];
	        this.reason = source["reason"];
	    }
	}
	export class UndoReport {
	    batch: string;
	    ok: number;
	    skipped: number;
	    failed: number;
	    items: UndoItem[];
	
	    static createFrom(source: any = {}) {
	        return new UndoReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.batch = source["batch"];
	        this.ok = source["ok"];
	        this.skipped = source["skipped"];
	        this.failed = source["failed"];
	        this.items = this.convertValues(source["items"], UndoItem);
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

