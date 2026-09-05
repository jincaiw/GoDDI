import{E as e,g as t,k as n}from"./echarts-Cw2yHLaZ.js";import{A as r,Bt as i,Ft as a,Ht as o,Lt as s,Pt as c,Rt as l,Vt as u,dt as d,ft as f,zt as p}from"./auth-CYMAsBzV.js";import{D as m}from"./vue-core-BDTvi3xZ.js";import{t as h}from"./use-compitable-OuJoh8Ky.js";import{t as g}from"./get-slot-6kXJmSMP.js";import{h as _,st as v}from"./index-ChsYpqrV.js";function y(e,t=`default`,n=[]){let{children:r}=e;if(typeof r==`object`&&r&&!Array.isArray(r)){let e=r[t];if(typeof e==`function`)return e()}return n}var b=c([a(`descriptions`,{fontSize:`var(--n-font-size)`},[a(`descriptions-separator`,`
 display: inline-block;
 margin: 0 8px 0 2px;
 `),a(`descriptions-table-wrapper`,[a(`descriptions-table`,[a(`descriptions-table-row`,[a(`descriptions-table-header`,{padding:`var(--n-th-padding)`}),a(`descriptions-table-content`,{padding:`var(--n-td-padding)`})])])]),p(`bordered`,[a(`descriptions-table-wrapper`,[a(`descriptions-table`,[a(`descriptions-table-row`,[c(`&:last-child`,[a(`descriptions-table-content`,{paddingBottom:0})])])])])]),l(`left-label-placement`,[a(`descriptions-table-content`,[c(`> *`,{verticalAlign:`top`})])]),l(`left-label-align`,[c(`th`,{textAlign:`left`})]),l(`center-label-align`,[c(`th`,{textAlign:`center`})]),l(`right-label-align`,[c(`th`,{textAlign:`right`})]),l(`bordered`,[a(`descriptions-table-wrapper`,`
 border-radius: var(--n-border-radius);
 overflow: hidden;
 background: var(--n-merged-td-color);
 border: 1px solid var(--n-merged-border-color);
 `,[a(`descriptions-table`,[a(`descriptions-table-row`,[c(`&:not(:last-child)`,[a(`descriptions-table-content`,{borderBottom:`1px solid var(--n-merged-border-color)`}),a(`descriptions-table-header`,{borderBottom:`1px solid var(--n-merged-border-color)`})]),a(`descriptions-table-header`,`
 font-weight: 400;
 background-clip: padding-box;
 background-color: var(--n-merged-th-color);
 `,[c(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})]),a(`descriptions-table-content`,[c(`&:not(:last-child)`,{borderRight:`1px solid var(--n-merged-border-color)`})])])])])]),a(`descriptions-header`,`
 font-weight: var(--n-th-font-weight);
 font-size: 18px;
 transition: color .3s var(--n-bezier);
 line-height: var(--n-line-height);
 margin-bottom: 16px;
 color: var(--n-title-text-color);
 `),a(`descriptions-table-wrapper`,`
 transition:
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[a(`descriptions-table`,`
 width: 100%;
 border-collapse: separate;
 border-spacing: 0;
 box-sizing: border-box;
 `,[a(`descriptions-table-row`,`
 box-sizing: border-box;
 transition: border-color .3s var(--n-bezier);
 `,[a(`descriptions-table-header`,`
 font-weight: var(--n-th-font-weight);
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-th-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `),a(`descriptions-table-content`,`
 vertical-align: top;
 line-height: var(--n-line-height);
 display: table-cell;
 box-sizing: border-box;
 color: var(--n-td-text-color);
 transition:
 color .3s var(--n-bezier),
 background-color .3s var(--n-bezier),
 border-color .3s var(--n-bezier);
 `,[s(`content`,`
 transition: color .3s var(--n-bezier);
 display: inline-block;
 color: var(--n-td-text-color);
 `)]),s(`label`,`
 font-weight: var(--n-th-font-weight);
 transition: color .3s var(--n-bezier);
 display: inline-block;
 margin-right: 14px;
 color: var(--n-th-text-color);
 `)])])])]),a(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color);
 --n-merged-td-color: var(--n-td-color);
 --n-merged-border-color: var(--n-border-color);
 `),u(a(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-modal);
 --n-merged-td-color: var(--n-td-color-modal);
 --n-merged-border-color: var(--n-border-color-modal);
 `)),o(a(`descriptions-table-wrapper`,`
 --n-merged-th-color: var(--n-th-color-popover);
 --n-merged-td-color: var(--n-td-color-popover);
 --n-merged-border-color: var(--n-border-color-popover);
 `))]),x=`DESCRIPTION_ITEM_FLAG`;function S(e){return typeof e==`object`&&e&&!Array.isArray(e)?e.type&&e.type.DESCRIPTION_ITEM_FLAG:!1}var C=e({name:`Descriptions`,props:Object.assign(Object.assign({},r.props),{title:String,column:{type:Number,default:3},columns:Number,labelPlacement:{type:String,default:`top`},labelAlign:{type:String,default:`left`},separator:{type:String,default:`:`},size:String,bordered:Boolean,labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]}),slots:Object,setup(e){let{mergedClsPrefixRef:n,inlineThemeDisabled:a,mergedComponentPropsRef:o}=f(e),s=t(()=>e.size||o?.value?.Descriptions?.size||`medium`),c=r(`Descriptions`,`-descriptions`,b,_,e,n),l=t(()=>{let{bordered:t}=e,n=s.value,{common:{cubicBezierEaseInOut:r},self:{titleTextColor:a,thColor:o,thColorModal:l,thColorPopover:u,thTextColor:d,thFontWeight:f,tdTextColor:p,tdColor:m,tdColorModal:h,tdColorPopover:g,borderColor:_,borderColorModal:v,borderColorPopover:y,borderRadius:b,lineHeight:x,[i(`fontSize`,n)]:S,[i(t?`thPaddingBordered`:`thPadding`,n)]:C,[i(t?`tdPaddingBordered`:`tdPadding`,n)]:w}}=c.value;return{"--n-title-text-color":a,"--n-th-padding":C,"--n-td-padding":w,"--n-font-size":S,"--n-bezier":r,"--n-th-font-weight":f,"--n-line-height":x,"--n-th-text-color":d,"--n-td-text-color":p,"--n-th-color":o,"--n-th-color-modal":l,"--n-th-color-popover":u,"--n-td-color":m,"--n-td-color-modal":h,"--n-td-color-popover":g,"--n-border-radius":b,"--n-border-color":_,"--n-border-color-modal":v,"--n-border-color-popover":y}}),u=a?d(`descriptions`,t(()=>{let t=``,{bordered:n}=e;return n&&(t+=`a`),t+=s.value[0],t}),l,e):void 0;return{mergedClsPrefix:n,cssVars:a?void 0:l,themeClass:u?.themeClass,onRender:u?.onRender,compitableColumn:h(e,[`columns`,`column`]),inlineThemeDisabled:a,mergedSize:s}},render(){let e=this.$slots.default,t=e?v(e()):[];t.length;let{contentClass:r,labelClass:i,compitableColumn:a,labelPlacement:o,labelAlign:s,mergedSize:c,bordered:l,title:u,cssVars:d,mergedClsPrefix:f,separator:p,onRender:h}=this;h?.();let _=t.filter(e=>S(e)),b=_.reduce((e,t,s)=>{let c=t.props||{},u=_.length-1===s,d=[`label`in c?c.label:y(t,`label`)],m=[y(t)],h=c.span||1,g=e.span;e.span+=h;let v=c.labelStyle||c[`label-style`]||this.labelStyle,b=c.contentStyle||c[`content-style`]||this.contentStyle;if(o===`left`)l?e.row.push(n(`th`,{class:[`${f}-descriptions-table-header`,i],colspan:1,style:v},d),n(`td`,{class:[`${f}-descriptions-table-content`,r],colspan:u?(a-g)*2+1:h*2-1,style:b},m)):e.row.push(n(`td`,{class:`${f}-descriptions-table-content`,colspan:u?(a-g)*2:h*2},n(`span`,{class:[`${f}-descriptions-table-content__label`,i],style:v},[...d,p&&n(`span`,{class:`${f}-descriptions-separator`},p)]),n(`span`,{class:[`${f}-descriptions-table-content__content`,r],style:b},m)));else{let t=u?(a-g)*2:h*2;e.row.push(n(`th`,{class:[`${f}-descriptions-table-header`,i],colspan:t,style:v},d)),e.secondRow.push(n(`td`,{class:[`${f}-descriptions-table-content`,r],colspan:t,style:b},m))}return(e.span>=a||u)&&(e.span=0,e.row.length&&(e.rows.push(e.row),e.row=[]),o!==`left`&&e.secondRow.length&&(e.rows.push(e.secondRow),e.secondRow=[])),e},{span:0,row:[],secondRow:[],rows:[]}).rows.map(e=>n(`tr`,{class:`${f}-descriptions-table-row`},e));return n(`div`,{style:d,class:[`${f}-descriptions`,this.themeClass,`${f}-descriptions--${o}-label-placement`,`${f}-descriptions--${s}-label-align`,`${f}-descriptions--${c}-size`,l&&`${f}-descriptions--bordered`]},u||this.$slots.header?n(`div`,{class:`${f}-descriptions-header`},u||g(this,`header`)):null,n(`div`,{class:`${f}-descriptions-table-wrapper`},n(`table`,{class:`${f}-descriptions-table`},n(`tbody`,null,o===`top`&&n(`tr`,{class:`${f}-descriptions-table-row`,style:{visibility:`collapse`}},m(a*2,n(`td`,null))),b))))}}),w={label:String,span:{type:Number,default:1},labelClass:String,labelStyle:[Object,String],contentClass:String,contentStyle:[Object,String]},T=e({name:`DescriptionsItem`,[x]:!0,props:w,slots:Object,render(){return null}});export{C as n,T as t};