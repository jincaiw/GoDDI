import{E as e,g as t,k as n}from"./vue.runtime.esm-bundler-B46jYzg4.js";import{Fn as r,Kr as i,Pn as a,Rr as o,Ur as s,Vr as c,er as l,gn as u,in as d,mn as f,nn as p,on as m,rn as h,zr as g}from"./router-BIZtGF2g.js";import{t as _}from"./light-DoLncLd0.js";var v={success:n(h,null),error:n(m,null),warning:n(p,null),info:n(d,null)},y=e({name:`ProgressCircle`,props:{clsPrefix:{type:String,required:!0},status:{type:String,required:!0},strokeWidth:{type:Number,required:!0},fillColor:[String,Object],railColor:String,railStyle:[String,Object],percentage:{type:Number,default:0},offsetDegree:{type:Number,default:0},showIndicator:{type:Boolean,required:!0},indicatorTextColor:String,unit:String,viewBoxWidth:{type:Number,required:!0},gapDegree:{type:Number,required:!0},gapOffsetDegree:{type:Number,default:0}},setup(e,{slots:r}){let a=t(()=>{let t=`gradient`,{fillColor:n}=e;return typeof n==`object`?`${t}-${i(JSON.stringify(n))}`:t});function o(t,n,r,i){let{gapDegree:o,viewBoxWidth:s,strokeWidth:c}=e,l=50+c/2,u=`M ${l},${l} m 0,50
      a 50,50 0 1 1 0,-100
      a 50,50 0 1 1 0,100`,d=Math.PI*2*50;return{pathString:u,pathStyle:{stroke:i===`rail`?r:typeof e.fillColor==`object`?`url(#${a.value})`:r,strokeDasharray:`${Math.min(t,100)/100*(d-o)}px ${s*8}px`,strokeDashoffset:`-${o/2}px`,transformOrigin:n?`center`:void 0,transform:n?`rotate(${n}deg)`:void 0}}}let s=()=>{let t=typeof e.fillColor==`object`,r=t?e.fillColor.stops[0]:``,i=t?e.fillColor.stops[1]:``;return t&&n(`defs`,null,n(`linearGradient`,{id:a.value,x1:`0%`,y1:`100%`,x2:`100%`,y2:`0%`},n(`stop`,{offset:`0%`,"stop-color":r}),n(`stop`,{offset:`100%`,"stop-color":i})))};return()=>{let{fillColor:t,railColor:i,strokeWidth:a,offsetDegree:c,status:l,percentage:u,showIndicator:d,indicatorTextColor:p,unit:m,gapOffsetDegree:h,clsPrefix:g}=e,{pathString:_,pathStyle:y}=o(100,0,i,`rail`),{pathString:b,pathStyle:x}=o(u,c,t,`fill`),S=100+a;return n(`div`,{class:`${g}-progress-content`,role:`none`},n(`div`,{class:`${g}-progress-graph`,"aria-hidden":!0},n(`div`,{class:`${g}-progress-graph-circle`,style:{transform:h?`rotate(${h}deg)`:void 0}},n(`svg`,{viewBox:`0 0 ${S} ${S}`},s(),n(`g`,null,n(`path`,{class:`${g}-progress-graph-circle-rail`,d:_,"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:y})),n(`g`,null,n(`path`,{class:[`${g}-progress-graph-circle-fill`,u===0&&`${g}-progress-graph-circle-fill--empty`],d:b,"stroke-width":a,"stroke-linecap":`round`,fill:`none`,style:x}))))),d?n(`div`,null,r.default?n(`div`,{class:`${g}-progress-custom-content`,role:`none`},r.default()):l===`default`?n(`div`,{class:`${g}-progress-text`,style:{color:p},role:`none`},n(`span`,{class:`${g}-progress-text__percentage`},u),n(`span`,{class:`${g}-progress-text__unit`},m)):n(`div`,{class:`${g}-progress-icon`,"aria-hidden":!0},n(f,{clsPrefix:g},{default:()=>v[l]}))):null)}}}),b={success:n(h,null),error:n(m,null),warning:n(p,null),info:n(d,null)},x=e({name:`ProgressLine`,props:{clsPrefix:{type:String,required:!0},percentage:{type:Number,default:0},railColor:String,railStyle:[String,Object],fillColor:[String,Object],status:{type:String,required:!0},indicatorPlacement:{type:String,required:!0},indicatorTextColor:String,unit:{type:String,default:`%`},processing:{type:Boolean,required:!0},showIndicator:{type:Boolean,required:!0},height:[String,Number],railBorderRadius:[String,Number],fillBorderRadius:[String,Number]},setup(e,{slots:r}){let i=t(()=>l(e.height)),a=t(()=>typeof e.fillColor==`object`?`linear-gradient(to right, ${e.fillColor?.stops[0]} , ${e.fillColor?.stops[1]})`:e.fillColor),o=t(()=>e.railBorderRadius===void 0?e.height===void 0?``:l(e.height,{c:.5}):l(e.railBorderRadius)),s=t(()=>e.fillBorderRadius===void 0?e.railBorderRadius===void 0?e.height===void 0?``:l(e.height,{c:.5}):l(e.railBorderRadius):l(e.fillBorderRadius));return()=>{let{indicatorPlacement:t,railColor:c,railStyle:l,percentage:u,unit:d,indicatorTextColor:p,status:m,showIndicator:h,processing:g,clsPrefix:_}=e;return n(`div`,{class:`${_}-progress-content`,role:`none`},n(`div`,{class:`${_}-progress-graph`,"aria-hidden":!0},n(`div`,{class:[`${_}-progress-graph-line`,{[`${_}-progress-graph-line--indicator-${t}`]:!0}]},n(`div`,{class:`${_}-progress-graph-line-rail`,style:[{backgroundColor:c,height:i.value,borderRadius:o.value},l]},n(`div`,{class:[`${_}-progress-graph-line-fill`,g&&`${_}-progress-graph-line-fill--processing`],style:{maxWidth:`${e.percentage}%`,background:a.value,height:i.value,lineHeight:i.value,borderRadius:s.value}},t===`inside`?n(`div`,{class:`${_}-progress-graph-line-indicator`,style:{color:p}},r.default?r.default():`${u}${d}`):null)))),h&&t===`outside`?n(`div`,null,r.default?n(`div`,{class:`${_}-progress-custom-content`,style:{color:p},role:`none`},r.default()):m===`default`?n(`div`,{role:`none`,class:`${_}-progress-icon ${_}-progress-icon--as-text`,style:{color:p}},u,d):n(`div`,{class:`${_}-progress-icon`,"aria-hidden":!0},n(f,{clsPrefix:_},{default:()=>b[m]}))):null)}}});function S(e,t,n=100){return`m ${n/2} ${n/2-e} a ${e} ${e} 0 1 1 0 ${2*e} a ${e} ${e} 0 1 1 0 -${2*e}`}var C=e({name:`ProgressMultipleCircle`,props:{clsPrefix:{type:String,required:!0},viewBoxWidth:{type:Number,required:!0},percentage:{type:Array,default:[0]},strokeWidth:{type:Number,required:!0},circleGap:{type:Number,required:!0},showIndicator:{type:Boolean,required:!0},fillColor:{type:Array,default:()=>[]},railColor:{type:Array,default:()=>[]},railStyle:{type:Array,default:()=>[]}},setup(e,{slots:r}){let i=t(()=>e.percentage.map((t,n)=>`${Math.PI*t/100*(e.viewBoxWidth/2-e.strokeWidth/2*(1+2*n)-e.circleGap*n)*2}, ${e.viewBoxWidth*8}`)),a=(t,r)=>{let i=e.fillColor[r],a=typeof i==`object`?i.stops[0]:``,o=typeof i==`object`?i.stops[1]:``;return typeof e.fillColor[r]==`object`&&n(`linearGradient`,{id:`gradient-${r}`,x1:`100%`,y1:`0%`,x2:`0%`,y2:`100%`},n(`stop`,{offset:`0%`,"stop-color":a}),n(`stop`,{offset:`100%`,"stop-color":o}))};return()=>{let{viewBoxWidth:t,strokeWidth:o,circleGap:s,showIndicator:c,fillColor:l,railColor:u,railStyle:d,percentage:f,clsPrefix:p}=e;return n(`div`,{class:`${p}-progress-content`,role:`none`},n(`div`,{class:`${p}-progress-graph`,"aria-hidden":!0},n(`div`,{class:`${p}-progress-graph-circle`},n(`svg`,{viewBox:`0 0 ${t} ${t}`},n(`defs`,null,f.map((e,t)=>a(e,t))),f.map((e,r)=>n(`g`,{key:r},n(`path`,{class:`${p}-progress-graph-circle-rail`,d:S(t/2-o/2*(1+2*r)-s*r,o,t),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:[{strokeDashoffset:0,stroke:u[r]},d[r]]}),n(`path`,{class:[`${p}-progress-graph-circle-fill`,e===0&&`${p}-progress-graph-circle-fill--empty`],d:S(t/2-o/2*(1+2*r)-s*r,o,t),"stroke-width":o,"stroke-linecap":`round`,fill:`none`,style:{strokeDasharray:i.value[r],strokeDashoffset:0,stroke:typeof l[r]==`object`?`url(#gradient-${r})`:l[r]}})))))),c&&r.default?n(`div`,null,n(`div`,{class:`${p}-progress-text`},r.default())):null)}}}),w=o([g(`progress`,{display:`inline-block`},[g(`progress-icon`,`
 color: var(--n-icon-color);
 transition: color .3s var(--n-bezier);
 `),c(`line`,`
 width: 100%;
 display: block;
 `,[g(`progress-content`,`
 display: flex;
 align-items: center;
 `,[g(`progress-graph`,{flex:1})]),g(`progress-custom-content`,{marginLeft:`14px`}),g(`progress-icon`,`
 width: 30px;
 padding-left: 14px;
 height: var(--n-icon-size-line);
 line-height: var(--n-icon-size-line);
 font-size: var(--n-icon-size-line);
 `,[c(`as-text`,`
 color: var(--n-text-color-line-outer);
 text-align: center;
 width: 40px;
 font-size: var(--n-font-size);
 padding-left: 4px;
 transition: color .3s var(--n-bezier);
 `)])]),c(`circle, dashboard`,{width:`120px`},[g(`progress-custom-content`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 `),g(`progress-text`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: inherit;
 font-size: var(--n-font-size-circle);
 color: var(--n-text-color-circle);
 font-weight: var(--n-font-weight-circle);
 transition: color .3s var(--n-bezier);
 white-space: nowrap;
 `),g(`progress-icon`,`
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 color: var(--n-icon-color);
 font-size: var(--n-icon-size-circle);
 `)]),c(`multiple-circle`,`
 width: 200px;
 color: inherit;
 `,[g(`progress-text`,`
 font-weight: var(--n-font-weight-circle);
 color: var(--n-text-color-circle);
 position: absolute;
 left: 50%;
 top: 50%;
 transform: translateX(-50%) translateY(-50%);
 display: flex;
 align-items: center;
 justify-content: center;
 transition: color .3s var(--n-bezier);
 `)]),g(`progress-content`,{position:`relative`}),g(`progress-graph`,{position:`relative`},[g(`progress-graph-circle`,[o(`svg`,{verticalAlign:`bottom`}),g(`progress-graph-circle-fill`,`
 stroke: var(--n-fill-color);
 transition:
 opacity .3s var(--n-bezier),
 stroke .3s var(--n-bezier),
 stroke-dasharray .3s var(--n-bezier);
 `,[c(`empty`,{opacity:0})]),g(`progress-graph-circle-rail`,`
 transition: stroke .3s var(--n-bezier);
 overflow: hidden;
 stroke: var(--n-rail-color);
 `)]),g(`progress-graph-line`,[c(`indicator-inside`,[g(`progress-graph-line-rail`,`
 height: 16px;
 line-height: 16px;
 border-radius: 10px;
 `,[g(`progress-graph-line-fill`,`
 height: inherit;
 border-radius: 10px;
 `),g(`progress-graph-line-indicator`,`
 background: #0000;
 white-space: nowrap;
 text-align: right;
 margin-left: 14px;
 margin-right: 14px;
 height: inherit;
 font-size: 12px;
 color: var(--n-text-color-line-inner);
 transition: color .3s var(--n-bezier);
 `)])]),c(`indicator-inside-label`,`
 height: 16px;
 display: flex;
 align-items: center;
 `,[g(`progress-graph-line-rail`,`
 flex: 1;
 transition: background-color .3s var(--n-bezier);
 `),g(`progress-graph-line-indicator`,`
 background: var(--n-fill-color);
 font-size: 12px;
 transform: translateZ(0);
 display: flex;
 vertical-align: middle;
 height: 16px;
 line-height: 16px;
 padding: 0 10px;
 border-radius: 10px;
 position: absolute;
 white-space: nowrap;
 color: var(--n-text-color-line-inner);
 transition:
 right .2s var(--n-bezier),
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 `)]),g(`progress-graph-line-rail`,`
 position: relative;
 overflow: hidden;
 height: var(--n-rail-height);
 border-radius: 5px;
 background-color: var(--n-rail-color);
 transition: background-color .3s var(--n-bezier);
 `,[g(`progress-graph-line-fill`,`
 background: var(--n-fill-color);
 position: relative;
 border-radius: 5px;
 height: inherit;
 width: 100%;
 max-width: 0%;
 transition:
 background-color .3s var(--n-bezier),
 max-width .2s var(--n-bezier);
 `,[c(`processing`,[o(`&::after`,`
 content: "";
 background-image: var(--n-line-bg-processing);
 animation: progress-processing-animation 2s var(--n-bezier) infinite;
 `)])])])])])]),o(`@keyframes progress-processing-animation`,`
 0% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 100%;
 opacity: 1;
 }
 66% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
 100% {
 position: absolute;
 left: 0;
 top: 0;
 bottom: 0;
 right: 0;
 opacity: 0;
 }
 `)]),T=e({name:`Progress`,props:Object.assign(Object.assign({},u.props),{processing:Boolean,type:{type:String,default:`line`},gapDegree:Number,gapOffsetDegree:Number,status:{type:String,default:`default`},railColor:[String,Array],railStyle:[String,Array],color:[String,Array,Object],viewBoxWidth:{type:Number,default:100},strokeWidth:{type:Number,default:7},percentage:[Number,Array],unit:{type:String,default:`%`},showIndicator:{type:Boolean,default:!0},indicatorPosition:{type:String,default:`outside`},indicatorPlacement:{type:String,default:`outside`},indicatorTextColor:String,circleGap:{type:Number,default:1},height:Number,borderRadius:[String,Number],fillBorderRadius:[String,Number],offsetDegree:Number}),setup(e){let n=t(()=>e.indicatorPlacement||e.indicatorPosition),i=t(()=>{if(e.gapDegree||e.gapDegree===0)return e.gapDegree;if(e.type===`dashboard`)return 75}),{mergedClsPrefixRef:o,inlineThemeDisabled:c}=r(e),l=u(`Progress`,`-progress`,w,_,e,o),d=t(()=>{let{status:t}=e,{common:{cubicBezierEaseInOut:n},self:{fontSize:r,fontSizeCircle:i,railColor:a,railHeight:o,iconSizeCircle:c,iconSizeLine:u,textColorCircle:d,textColorLineInner:f,textColorLineOuter:p,lineBgProcessing:m,fontWeightCircle:h,[s(`iconColor`,t)]:g,[s(`fillColor`,t)]:_}}=l.value;return{"--n-bezier":n,"--n-fill-color":_,"--n-font-size":r,"--n-font-size-circle":i,"--n-font-weight-circle":h,"--n-icon-color":g,"--n-icon-size-circle":c,"--n-icon-size-line":u,"--n-line-bg-processing":m,"--n-rail-color":a,"--n-rail-height":o,"--n-text-color-circle":d,"--n-text-color-line-inner":f,"--n-text-color-line-outer":p}}),f=c?a(`progress`,t(()=>e.status[0]),d,e):void 0;return{mergedClsPrefix:o,mergedIndicatorPlacement:n,gapDeg:i,cssVars:c?void 0:d,themeClass:f?.themeClass,onRender:f?.onRender}},render(){let{type:e,cssVars:t,indicatorTextColor:r,showIndicator:i,status:a,railColor:o,railStyle:s,color:c,percentage:l,viewBoxWidth:u,strokeWidth:d,mergedIndicatorPlacement:f,unit:p,borderRadius:m,fillBorderRadius:h,height:g,processing:_,circleGap:v,mergedClsPrefix:b,gapDeg:S,gapOffsetDegree:w,themeClass:T,$slots:E,onRender:D}=this;return D?.(),n(`div`,{class:[T,`${b}-progress`,`${b}-progress--${e}`,`${b}-progress--${a}`],style:t,"aria-valuemax":100,"aria-valuemin":0,"aria-valuenow":l,role:e===`circle`||e===`line`||e===`dashboard`?`progressbar`:`none`},e===`circle`||e===`dashboard`?n(y,{clsPrefix:b,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,offsetDegree:this.offsetDegree,percentage:l,viewBoxWidth:u,strokeWidth:d,gapDegree:S===void 0?e===`dashboard`?75:0:S,gapOffsetDegree:w,unit:p},E):e===`line`?n(x,{clsPrefix:b,status:a,showIndicator:i,indicatorTextColor:r,railColor:o,fillColor:c,railStyle:s,percentage:l,processing:_,indicatorPlacement:f,unit:p,fillBorderRadius:h,railBorderRadius:m,height:g},E):e===`multiple-circle`?n(C,{clsPrefix:b,strokeWidth:d,railColor:o,fillColor:c,railStyle:s,viewBoxWidth:u,percentage:l,showIndicator:i,circleGap:v},E):null)}});export{T as t};