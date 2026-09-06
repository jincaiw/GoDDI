import{O as e,j as t,v as n}from"./echarts-eUEtiXc8.js";import{K as r,N as i,P as a,Y as o,Z as s,i as c}from"./light-BKCENzy2.js";import{t as l}from"./use-compitable-DSN36vHZ.js";import{n as u}from"./index-B8wbSRXy.js";var d=r(`text`,`
 transition: color .3s var(--n-bezier);
 color: var(--n-text-color);
`,[o(`strong`,`
 font-weight: var(--n-font-weight-strong);
 `),o(`italic`,{fontStyle:`italic`}),o(`underline`,{textDecoration:`underline`}),o(`code`,`
 line-height: 1.4;
 display: inline-block;
 font-family: var(--n-font-famliy-mono);
 transition: 
 color .3s var(--n-bezier),
 border-color .3s var(--n-bezier),
 background-color .3s var(--n-bezier);
 box-sizing: border-box;
 padding: .05em .35em 0 .35em;
 border-radius: var(--n-code-border-radius);
 font-size: .9em;
 color: var(--n-code-text-color);
 background-color: var(--n-code-color);
 border: var(--n-code-border);
 `)]),f=e({name:`Text`,props:Object.assign(Object.assign({},c.props),{code:Boolean,type:{type:String,default:`default`},delete:Boolean,strong:Boolean,italic:Boolean,underline:Boolean,depth:[String,Number],tag:String,as:{type:String,validator:()=>!0,default:void 0}}),setup(e){let{mergedClsPrefixRef:t,inlineThemeDisabled:r}=a(e),o=c(`Typography`,`-text`,d,u,e,t),f=n(()=>{let{depth:t,type:n}=e,r=n==="default"?t===void 0?`textColor`:`textColor${t}Depth`:s(`textColor`,n),{common:{fontWeightStrong:i,fontFamilyMono:a,cubicBezierEaseInOut:c},self:{codeTextColor:l,codeBorderRadius:u,codeColor:d,codeBorder:f,[r]:p}}=o.value;return{"--n-bezier":c,"--n-text-color":p,"--n-font-weight-strong":i,"--n-font-famliy-mono":a,"--n-code-border-radius":u,"--n-code-text-color":l,"--n-code-color":d,"--n-code-border":f}}),p=r?i(`text`,n(()=>`${e.type[0]}${e.depth||``}`),f,e):void 0;return{mergedClsPrefix:t,compitableTag:l(e,[`as`,`tag`]),cssVars:r?void 0:f,themeClass:p?.themeClass,onRender:p?.onRender}},render(){var e,n;let{mergedClsPrefix:r}=this;(e=this.onRender)==null||e.call(this);let i=[`${r}-text`,this.themeClass,{[`${r}-text--code`]:this.code,[`${r}-text--delete`]:this.delete,[`${r}-text--strong`]:this.strong,[`${r}-text--italic`]:this.italic,[`${r}-text--underline`]:this.underline}],a=(n=this.$slots).default?.call(n);return this.code?t(`code`,{class:i,style:this.cssVars},this.delete?t(`del`,null,a):a):this.delete?t(`del`,{class:i,style:this.cssVars},a):t(this.compitableTag||`span`,{class:i,style:this.cssVars},a)}});export{f as t};