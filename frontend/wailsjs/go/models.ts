export namespace main {
	
	export class Frozen {
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new Frozen(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class GuideDetail {
	    guide: store.Guide;
	    steps: store.Step[];
	    deleted: store.Step[];
	
	    static createFrom(source: any = {}) {
	        return new GuideDetail(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.guide = this.convertValues(source["guide"], store.Guide);
	        this.steps = this.convertValues(source["steps"], store.Step);
	        this.deleted = this.convertValues(source["deleted"], store.Step);
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
	export class RenderJob {
	    stepId: number;
	    imageSha: string;
	    annotationsJson: string;
	    viewJson: string;
	    key: string;
	    cached: boolean;
	
	    static createFrom(source: any = {}) {
	        return new RenderJob(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.stepId = source["stepId"];
	        this.imageSha = source["imageSha"];
	        this.annotationsJson = source["annotationsJson"];
	        this.viewJson = source["viewJson"];
	        this.key = source["key"];
	        this.cached = source["cached"];
	    }
	}

}

export namespace store {
	
	export class Folder {
	    id: number;
	    title: string;
	
	    static createFrom(source: any = {}) {
	        return new Folder(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	    }
	}
	export class Guide {
	    id: number;
	    folderId: number;
	    title: string;
	    updatedAt: string;
	    stepCount: number;
	    thumbSha: string;
	
	    static createFrom(source: any = {}) {
	        return new Guide(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.folderId = source["folderId"];
	        this.title = source["title"];
	        this.updatedAt = source["updatedAt"];
	        this.stepCount = source["stepCount"];
	        this.thumbSha = source["thumbSha"];
	    }
	}
	export class Preset {
	    id: number;
	    name: string;
	    format: string;
	    options: string;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.format = source["format"];
	        this.options = source["options"];
	    }
	}
	export class Step {
	    id: number;
	    guideId: number;
	    parentId: number;
	    sort: number;
	    kind: string;
	    title: string;
	    description: string;
	    imageSha: string;
	    hidden: boolean;
	    deleted: boolean;
	    captureJson: string;
	    annotationsJson: string;
	    calloutsJson: string;
	    viewJson: string;
	
	    static createFrom(source: any = {}) {
	        return new Step(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.guideId = source["guideId"];
	        this.parentId = source["parentId"];
	        this.sort = source["sort"];
	        this.kind = source["kind"];
	        this.title = source["title"];
	        this.description = source["description"];
	        this.imageSha = source["imageSha"];
	        this.hidden = source["hidden"];
	        this.deleted = source["deleted"];
	        this.captureJson = source["captureJson"];
	        this.annotationsJson = source["annotationsJson"];
	        this.calloutsJson = source["calloutsJson"];
	        this.viewJson = source["viewJson"];
	    }
	}

}

